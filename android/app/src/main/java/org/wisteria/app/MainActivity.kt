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
        private const val REQ_EXPORT = 43
        private const val REQ_IMPORT = 44
        private const val TAG = "WisteriaUI"

        /** Set by the QS tile when it cannot start the tunnel itself (VPN
         *  consent pending / no active profile): the activity connects on
         *  launch instead. */
        const val EXTRA_AUTO_CONNECT = "org.wisteria.app.AUTO_CONNECT"

        /** Launcher-shortcut actions. */
        const val ACTION_SHORTCUT_TOGGLE = "org.wisteria.app.SHORTCUT_TOGGLE"
        const val ACTION_SHORTCUT_CONNECT = "org.wisteria.app.SHORTCUT_CONNECT"
    }

    private lateinit var web: WebView
    private lateinit var bridge: WisteriaBridge

    /** The FrameLayout that pads the system bars. Kept so a theme change can
     *  repaint the strips behind them (API 35 ignores window.statusBarColor
     *  under enforced edge-to-edge, so the view background IS the chrome). */
    private var rootView: android.widget.FrameLayout? = null

    /** This activity's own event emitter — onDestroy removes exactly it,
     *  never the whole registry: restartApp() creates the new activity
     *  BEFORE the old one is destroyed, and a blanket clear() wiped the
     *  fresh emitter (push events died until the next recreation). */
    private var emitter: ((String, String) -> Unit)? = null

    /** Latest bottom system inset in CSS px, mirrored to the page as
     *  --sat-bottom (see onCreate). -1 until the first insets dispatch. */
    @Volatile
    private var navInsetPx: Int = -1

    @Volatile
    private var pageReady = false

    /** (Re)injects the inset variable: the first insets dispatch usually
     *  lands before the page exists, and every page load wipes inline
     *  styles, so the value is pushed on insets change AND on load finish. */
    private fun pushSafeInsetJs() {
        val px = navInsetPx
        if (px < 0 || !pageReady || isFinishing || isDestroyed) return
        runOnUiThread {
            if (!isFinishing && !isDestroyed) {
                web.evaluateJavascript(
                    "document.documentElement.style.setProperty('--sat-bottom','${px}px')", null
                )
            }
        }
    }

    /** The strips left visible by the inset padding (status bar above the
     *  glass header, nav bar below it) must blend with them, not fight them.
     *  A light theme (arctic) needs light strips with DARK system icons —
     *  the old hardcoded near-black bars with light icons put white status
     *  icons on a white page. Called on theme changes too, since the web
     *  chrome re-themes via CSS. */
    fun applyThemeFromSettings() {
        val light = try {
            AndroidStore(this).settings().optString("theme") == "arctic"
        } catch (_: Throwable) {
            false
        }
        val bg = if (light) "#e8f0f8" else "#050505"
        try {
            val color = Color.parseColor(bg)
            window.statusBarColor = color
            window.navigationBarColor = color
            rootView?.setBackgroundColor(color)
            web.setBackgroundColor(color)
        } catch (_: Throwable) {
        }
        try {
            WindowCompat.getInsetsController(window, window.decorView).apply {
                isAppearanceLightStatusBars = light
                isAppearanceLightNavigationBars = light
            }
        } catch (_: Throwable) {
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        web = WebView(this)
        web.setBackgroundColor(Color.parseColor("#050505"))
        // No blue glow/edge shimmer on scroll: the SPA scrolls its own
        // container, so the overscroll effect only ever appeared as an
        // artifact over the glass panels.
        web.overScrollMode = android.view.View.OVER_SCROLL_NEVER
        web.settings.apply {
            javaScriptEnabled = true
            domStorageEnabled = true // the frontend keeps group-collapse state in localStorage
            // file:///android_asset loads regardless of this flag; leaving
            // file access on only widened what a script-injection could read.
            allowFileAccess = false
        }
        web.webViewClient = object : WebViewClient() {
            override fun onPageFinished(view: WebView, url: String?) {
                pageReady = true
                pushSafeInsetJs()
            }
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
        if (BuildConfig.DEBUG) WebView.setWebContentsDebuggingEnabled(true)
        // Edge-to-edge: with targetSdk 35 the window spans the whole screen
        // anyway, so the app goes under the system bars on purpose. The
        // frame is padded on the top/sides (status bar, cutout, navigation
        // side strips) so the header and content never hide under them,
        // while the bottom is left open: the webview reaches the physical
        // bottom edge, its viewport carries viewport-fit=cover, and the CSS
        // pads the bottom navigation with env(safe-area-inset-bottom). That
        // makes the nav bar's glass blend behind the gesture pill / nav
        // buttons instead of leaving a black strip under the app.
        // The IME inset lifts the content above the keyboard on API 30+,
        // where adjustResize no longer resizes the window.
        WindowCompat.setDecorFitsSystemWindows(window, false)
        val root = android.widget.FrameLayout(this)
        rootView = root
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
        // Insets are NOT consumed: Chromium's safe-area (env(safe-area-inset-*)
        // with viewport-fit=cover) is computed from the insets the WebView
        // itself receives, and a CONSUMED dispatch left env() at 0 — the nav
        // bar sat under the system buttons again. IME insets are zeroed in
        // the forwarded copy (the frame padding already lifts the content
        // above the keyboard; letting Chromium shrink the visual viewport
        // too would move the page up twice). The nav-bar inset is mirrored
        // into a CSS variable as a second belt: --sat-bottom, in CSS px.
        val density = resources.displayMetrics.density
        ViewCompat.setOnApplyWindowInsetsListener(window.decorView) { _, insets ->
            val bars = insets.getInsets(
                WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout()
            )
            val ime = insets.getInsets(WindowInsetsCompat.Type.ime())
            root.setPadding(bars.left, bars.top, bars.right, max(0, ime.bottom - bars.bottom))
            navInsetPx = (bars.bottom / density).toInt()
            pushSafeInsetJs()
            WindowInsetsCompat.Builder(insets)
                .setInsets(WindowInsetsCompat.Type.ime(), androidx.core.graphics.Insets.of(0, 0, 0, 0))
                .build()
        }
        setContentView(root)

        emitter = { event, payload ->
            // evaluateJavascript must run on the UI thread; skip silently
            // when the webview is gone (activity finishing).
            runOnUiThread {
                if (!isFinishing && !isDestroyed) {
                    // quote() the event name: a JSON string literal is a
                    // valid JS expression even if a future pushEvent uses a
                    // name with quotes; payload is already JSON or null.
                    // The guard: engine state events can fire while the
                    // page is still loading — before android-shim.js has
                    // defined __wisteriaEmit — and an uncaught TypeError
                    // just spams the console (the frontend re-pulls state
                    // on its 3s poll, so dropping an early event is safe).
                    web.evaluateJavascript(
                        "window.__wisteriaEmit && window.__wisteriaEmit(${JSONObject.quote(event)}, $payload)", null
                    )
                }
            }
        }
        EngineHub.emitters += emitter!!

        web.loadUrl("file:///android_asset/ui/index.html")
        applyThemeFromSettings()
        handleAutoConnect(intent)
        handleIntentExtras(intent)
        refreshShortcuts()

        // Android 13+ hides the VPN notification without the runtime grant;
        // denial is harmless (the foreground service still runs).
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            ActivityCompat.requestPermissions(this, arrayOf(Manifest.permission.POST_NOTIFICATIONS), REQ_NOTIF)
        }
    }

    /** The bridge asks for VPN consent mid-Connect; the retry follows here.
     *  Called from the WebView bridge thread — the dialog must be launched
     *  on the UI thread. The pending id lives in VpnConsentHolder so a
     *  recreation while the dialog is up does not lose the grant. */
    fun requestVpnConsent(profileId: String) {
        runOnUiThread {
            if (isFinishing || isDestroyed) return@runOnUiThread
            val intent = VpnService.prepare(this) ?: run {
                bridge.onVpnConsentGranted(profileId)
                return@runOnUiThread
            }
            startActivityForResult(intent, REQ_VPN)
        }
    }

    /** QS tile hand-off: consume the extra once so a recreation does not
     *  reconnect again, then run the normal connect (consent dialog if needed). */
    private fun handleAutoConnect(i: Intent?) {
        if (i?.getBooleanExtra(EXTRA_AUTO_CONNECT, false) != true) return
        i.removeExtra(EXTRA_AUTO_CONNECT)
        bridge.connectActive()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handleAutoConnect(intent)
        handleIntentExtras(intent)
    }

    /**
     * Deep links and launcher shortcuts. A share link opened from a browser
     * (`wisteria://…`, `vless://…`) is imported through the same bridge path
     * as the Links tab, then the SPA is told to show the Profiles tab. The
     * consumed action/URI is cleared so a rotation or a resume does not
     * import the link twice.
     */
    private fun handleIntentExtras(intent: Intent?) {
        if (intent == null) return
        val data = intent.data
        if (data != null) {
            intent.data = null
            val uri = data.toString()
            // Run off the UI thread: parsing + a store write must never
            // block the first frame.
            Thread {
                try {
                    val res = bridge.importDeepLink(uri)
                    val added = res.optJSONArray("added")?.length() ?: 0
                    EngineHub.pushEvent(
                        "deeplink",
                        JSONObject().put("added", added).put("ok", added > 0)
                    )
                } catch (t: Throwable) {
                    EngineHub.appLog("deep link: ${t.message}")
                    EngineHub.pushEvent("deeplink", JSONObject().put("added", 0).put("ok", false))
                }
            }.start()
            return
        }
        when (intent.action) {
            ACTION_SHORTCUT_TOGGLE -> {
                intent.action = null
                Thread { bridge.toggleFromShortcut() }.start()
            }
            ACTION_SHORTCUT_CONNECT -> {
                val id = intent.getStringExtra(WisteriaVpnService.EXTRA_PROFILE_ID).orEmpty()
                intent.action = null
                intent.removeExtra(WisteriaVpnService.EXTRA_PROFILE_ID)
                if (id.isNotEmpty()) {
                    Thread {
                        try {
                            bridge.connectFromShortcut(id)
                        } catch (t: Throwable) {
                            EngineHub.appLog("ярлык: ${t.message}")
                        }
                    }.start()
                }
            }
        }
    }

    /** SAF: write the backup the bridge produces into a user-picked file. */
    fun exportBackup() {
        runOnUiThread {
            if (isFinishing || isDestroyed) return@runOnUiThread
            val name = "wisteria-backup-" +
                java.text.SimpleDateFormat("yyyyMMdd-HHmm", java.util.Locale.US).format(java.util.Date()) + ".json"
            try {
                startActivityForResult(
                    Intent(Intent.ACTION_CREATE_DOCUMENT)
                        .addCategory(Intent.CATEGORY_OPENABLE)
                        .setType("application/json")
                        .putExtra(Intent.EXTRA_TITLE, name),
                    REQ_EXPORT,
                )
            } catch (t: Throwable) {
                EngineHub.appLog("экспорт: ${t.message}")
            }
        }
    }

    /** SAF: read a backup file the user picked and hand it to the bridge. */
    fun importBackup() {
        runOnUiThread {
            if (isFinishing || isDestroyed) return@runOnUiThread
            try {
                startActivityForResult(
                    Intent(Intent.ACTION_OPEN_DOCUMENT)
                        .addCategory(Intent.CATEGORY_OPENABLE)
                        .setType("*/*"),
                    REQ_IMPORT,
                )
            } catch (t: Throwable) {
                EngineHub.appLog("импорт: ${t.message}")
            }
        }
    }

    private fun writeBackup(uri: android.net.Uri?) {
        if (uri == null) return
        Thread {
            try {
                val json = bridge.backupJson()
                contentResolver.openOutputStream(uri)?.use { it.write(json.toByteArray(Charsets.UTF_8)) }
                EngineHub.appLog("резервная копия сохранена")
                EngineHub.pushEvent("backup", JSONObject().put("ok", true).put("action", "export"))
            } catch (t: Throwable) {
                EngineHub.appLog("экспорт: ${t.message}")
                EngineHub.pushEvent("backup", JSONObject().put("ok", false).put("action", "export").put("error", t.message))
            }
        }.start()
    }

    private fun readBackup(uri: android.net.Uri?) {
        if (uri == null) return
        Thread {
            try {
                val text = contentResolver.openInputStream(uri)?.use {
                    it.readBytes().toString(Charsets.UTF_8)
                } ?: ""
                if (text.isBlank()) throw IllegalStateException("пустой файл")
                val res = bridge.restoreBackup(text)
                EngineHub.pushEvent(
                    "backup",
                    JSONObject().put("ok", true).put("action", "import")
                        .put("profiles", res.optInt("profiles")).put("subs", res.optInt("subs"))
                )
            } catch (t: Throwable) {
                EngineHub.appLog("импорт: ${t.message}")
                EngineHub.pushEvent("backup", JSONObject().put("ok", false).put("action", "import").put("error", t.message))
            }
        }.start()
    }

    /**
     * Launcher shortcuts: one toggle plus up to two most recent profiles.
     * Re-registered on every launch (the profile list changes), which is
     * also how the "recent" ordering stays right.
     */
    private fun refreshShortcuts() {
        try {
            val icon = androidx.core.graphics.drawable.IconCompat.createWithResource(this, R.mipmap.ic_launcher)
            val list = ArrayList<androidx.core.content.pm.ShortcutInfoCompat>()
            list.add(
                androidx.core.content.pm.ShortcutInfoCompat.Builder(this, "toggle")
                    .setShortLabel(getString(R.string.shortcut_toggle))
                    .setIcon(icon)
                    .setIntent(
                        Intent(this, MainActivity::class.java)
                            .setAction(ACTION_SHORTCUT_TOGGLE)
                            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                    )
                    .build()
            )
            val profiles = AndroidStore(this).listProfiles()
            var n = 0
            for (i in 0 until profiles.length()) {
                if (n >= 2) break
                val p = profiles.getJSONObject(i)
                val id = p.optString("id")
                if (id.isEmpty()) continue
                val name = p.optString("name").ifEmpty { p.optString("host") }.take(40)
                list.add(
                    androidx.core.content.pm.ShortcutInfoCompat.Builder(this, "p$id")
                        .setShortLabel(name)
                        .setIcon(icon)
                        .setIntent(
                            Intent(this, MainActivity::class.java)
                                .setAction(ACTION_SHORTCUT_CONNECT)
                                .putExtra(WisteriaVpnService.EXTRA_PROFILE_ID, id)
                                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                        )
                        .build()
                )
                n++
            }
            androidx.core.content.pm.ShortcutManagerCompat.setDynamicShortcuts(this, list)
        } catch (t: Throwable) {
            android.util.Log.w(TAG, "shortcuts", t)
        }
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        when (requestCode) {
            REQ_EXPORT -> if (resultCode == RESULT_OK) writeBackup(data?.data)
            REQ_IMPORT -> if (resultCode == RESULT_OK) readBackup(data?.data)
        }
        if (requestCode == REQ_VPN) {
            val id = VpnConsentHolder.pendingId
            VpnConsentHolder.pendingId = null
            if (resultCode == RESULT_OK && id != null) {
                bridge.onVpnConsentGranted(id)
            } else {
                // Dismissing the system dialog must not leave the optimistic
                // "CONNECTING…" screen up: the service never starts in this
                // case, so push the reset ourselves.
                EngineHub.appLog("согласие на VPN не выдано")
                EngineHub.pushEvent("state", WisteriaBridge.staticStatusJson("disconnected", ""))
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

    /** Evaluate JS on the UI thread; used by WisteriaBridge.CallAsync() to
     *  resolve a pending promise without touching the WebView off-thread. */
    fun evalJs(js: String) {
        runOnUiThread {
            if (isFinishing || isDestroyed) return@runOnUiThread
            try {
                web.evaluateJavascript(js, null)
            } catch (_: Throwable) {
            }
        }
    }

    override fun onDestroy() {
        // Remove ONLY this activity's emitter and holder slot: with
        // restartApp() the successor is already registered when this runs,
        // and a blanket clear()/null used to kill its event feed.
        emitter?.let { EngineHub.emitters -= it }
        emitter = null
        if (WisteriaBridgeHolder.instance === bridge) {
            WisteriaBridgeHolder.instance = null
        }
        // The JS interface holds this activity through the bridge; detach it
        // before the webview dies so a stale page can never reach a dead
        // activity (rotation recreates the whole pair anyway).
        try {
            web.removeJavascriptInterface("WisteriaAndroid")
            web.destroy()
        } catch (_: Throwable) {
        }
        super.onDestroy()
    }
}
