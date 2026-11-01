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
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.appcompat.app.AppCompatActivity
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import kotlin.math.max

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

    /** Theme accents, mirroring the frontend THEMES list (app.js). */
    private val THEME_ACCENTS = mapOf(
        "lilac" to "#a78bfa",
        "neon-dark" to "#00e8a2",
        "warm-sand" to "#e8a040",
        "arctic" to "#2a64b4",
        "sunset" to "#f06888",
        "matrix" to "#00ff41",
        "mono" to "#e02020",
    )

    /** The system status bar must follow the app theme (the WebView
     *  chrome is themed by CSS; this bar belongs to Android). */
    fun applyThemeFromSettings() {
        try {
            val theme = AndroidStore(this).settings().optString("theme", "lilac")
            window.statusBarColor = Color.parseColor(THEME_ACCENTS[theme] ?: "#a78bfa")
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
        web.webViewClient = WebViewClient()
        web.webChromeClient = object : WebChromeClient() {
            override fun onConsoleMessage(m: android.webkit.ConsoleMessage): Boolean {
                Log.d(TAG, "${m.message()} @${m.lineNumber()}")
                return true
            }
        }
        bridge = WisteriaBridge(this)
        WisteriaBridgeHolder.instance = bridge
        web.addJavascriptInterface(bridge, "WisteriaAndroid")
        setContentView(web)

        // Edge-to-edge ROMs draw the app under the status bar, which pushed
        // the logo under the clock; pad the webview by the system-bar insets
        // instead. The listener sits on the DECOR view — the dispatch origin
        // — because with targetSdk 35 (enforced edge-to-edge) insets that a
        // plain listener on the content view expects may never arrive (the
        // v1.0.0 release rendered the header under the clock). The IME inset
        // lifts the content above the keyboard on API 30+, where adjustResize
        // no longer resizes the window.
        ViewCompat.setOnApplyWindowInsetsListener(window.decorView) { _, insets ->
            val sys = insets.getInsets(WindowInsetsCompat.Type.systemBars())
            val ime = insets.getInsets(WindowInsetsCompat.Type.ime())
            web.setPadding(sys.left, sys.top, sys.right, max(sys.bottom, ime.bottom))
            WindowInsetsCompat.CONSUMED
        }

        EngineHub.emitters += { event, payload ->
            // evaluateJavascript must run on the UI thread; skip silently
            // when the webview is gone (activity finishing).
            runOnUiThread {
                if (!isFinishing && !isDestroyed) {
                    web.evaluateJavascript("window.__wisteriaEmit(\"$event\", $payload)", null)
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
