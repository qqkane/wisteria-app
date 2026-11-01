package org.wisteria.app

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.util.Log
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.appcompat.app.AppCompatActivity
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import kotlin.math.max
import org.json.JSONObject

/**
 * Hosts the shared frontend (the desktop «Наша Сеть» UI) from assets and
 * exposes the Kotlin bridge to it as window.WisteriaAndroid. The
 * android-shim.js loaded by the page wires that object into the
 * window.go/window.runtime shapes the frontend already speaks.
 */
class MainActivity : AppCompatActivity() {

    companion object {
        private const val REQ_VPN = 41
        private const val REQ_NOTIF = 42
        private const val TAG = "WisteriaUI"
    }

    private lateinit var web: WebView
    private lateinit var bridge: WisteriaBridge

    @Volatile
    private var pendingConnectId: String? = null

    /** The strips left visible by the inset padding (status bar above the
     *  glass header, nav bar below it) must blend with them, not fight them:
     *  the accent lives in the UI itself, so both bars stay near-black.
     *  Called on theme changes too — the web chrome re-themes via CSS. */
    fun applyThemeFromSettings() {
        try {
            window.statusBarColor = Color.parseColor("#050505")
            window.navigationBarColor = Color.parseColor("#050505")
        } catch (_: Throwable) {
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        web = WebView(this)
        web.setBackgroundColor(Color.parseColor("#050505"))
        web.settings.apply {
            javaScriptEnabled = true
            domStorageEnabled = true // the frontend keeps group-collapse state in localStorage
            // file:///android_asset loads regardless of this flag; leaving
            // file access on only widened what a script-injection could read.
            allowFileAccess = false
        }
        web.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                // Only the bundled asset page may render here: the JS bridge
                // is bound for it and must never be exposed to a remote
                // origin. Everything else (a future «скачать релиз» link,
                // help pages) leaves for the system browser.
                if (request.url.scheme == "file") return false
                try {
                    startActivity(Intent(Intent.ACTION_VIEW, request.url))
                } catch (_: Exception) {
                }
                return true
            }
        }
        web.webChromeClient = object : WebChromeClient() {
            override fun onConsoleMessage(m: android.webkit.ConsoleMessage): Boolean {
                Log.d(TAG, "${m.message()} @${m.lineNumber()}")
                return true
            }
        }
        bridge = WisteriaBridge(this)
        WisteriaBridgeHolder.instance = bridge
        web.addJavascriptInterface(bridge, "WisteriaAndroid")
        // Edge-to-edge ROMs draw the app under the system bars: with
        // targetSdk 35 (enforced edge-to-edge) the window spans the whole
        // screen, which put the header under the clock and the bottom nav
        // under the gesture pill. WebView ignores its own view padding, so
        // the v1.1.4 fix that padded the WebView directly did nothing —
        // wrap it in a frame and pad that, which insets the page viewport.
        // The IME inset lifts the content above the keyboard on API 30+,
        // where adjustResize no longer resizes the window.
        val root = android.widget.FrameLayout(this)
        // API 35 ignores window.statusBarColor under enforced edge-to-edge,
        // so the padded strips show the view background of the frame — keep
        // it the app near-black, and the system icons light on top of it.
        root.setBackgroundColor(Color.parseColor("#050505"))
        WindowCompat.getInsetsController(window, window.decorView).apply {
            isAppearanceLightStatusBars = false
            isAppearanceLightNavigationBars = false
        }
        root.addView(
            web,
            android.widget.FrameLayout.LayoutParams(
                android.widget.FrameLayout.LayoutParams.MATCH_PARENT,
                android.widget.FrameLayout.LayoutParams.MATCH_PARENT
            )
        )
        // The listener stays on the DECOR view — the dispatch origin —
        // because with targetSdk 35 insets that a plain content-view
        // listener expects may never arrive; the padding goes to the frame.
        ViewCompat.setOnApplyWindowInsetsListener(window.decorView) { _, insets ->
            val bars = insets.getInsets(
                WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout()
            )
            val ime = insets.getInsets(WindowInsetsCompat.Type.ime())
            root.setPadding(bars.left, bars.top, bars.right, max(bars.bottom, ime.bottom))
            WindowInsetsCompat.CONSUMED
        }
        setContentView(root)

        EngineHub.emitters += { event, payload ->
            // evaluateJavascript must run on the UI thread; skip silently
            // when the webview is gone (activity finishing).
            runOnUiThread {
                if (!isFinishing && !isDestroyed) {
                    // quote() the event name: a JSON string literal is a
                    // valid JS expression even if a future pushEvent uses a
                    // name with quotes; payload is already JSON or null.
                    web.evaluateJavascript(
                        "window.__wisteriaEmit(${JSONObject.quote(event)}, $payload)", null
                    )
                }
            }
        }

        web.loadUrl("file:///android_asset/ui/index.html")
        applyThemeFromSettings()

        // Android 13+ hides the VPN notification without the runtime grant;
        // denial is harmless (the foreground service still runs).
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            ActivityCompat.requestPermissions(this, arrayOf(Manifest.permission.POST_NOTIFICATIONS), REQ_NOTIF)
        }
    }

    /** The bridge asks for VPN consent mid-Connect; the retry follows here. */
    fun requestVpnConsent(profileId: String) {
        val intent = VpnService.prepare(this) ?: run {
            bridge.onVpnConsentGranted(profileId)
            return
        }
        pendingConnectId = profileId
        startActivityForResult(intent, REQ_VPN)
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == REQ_VPN) {
            val id = pendingConnectId
            pendingConnectId = null
            if (resultCode == RESULT_OK && id != null) {
                bridge.onVpnConsentGranted(id)
            } else {
                EngineHub.appLog("согласие на VPN не выдано")
            }
        }
    }

    @Deprecated("Deprecated in Java")
    override fun onBackPressed() {
        // The SPA has its own layers (open modals, a non-VPN tab): let the
        // shim consume Back before the activity finishes over a dialog.
        web.evaluateJavascript("window.__wisteriaBack ? window.__wisteriaBack() : 'false'") { handled ->
            if (handled != "\"true\"") super.onBackPressed()
        }
    }

    fun restartApp() {
        val intent = Intent(this, MainActivity::class.java)
        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)
        startActivity(intent)
        finishAffinity()
    }

    override fun onDestroy() {
        EngineHub.emitters.clear()
        WisteriaBridgeHolder.instance = null
        super.onDestroy()
    }
}
