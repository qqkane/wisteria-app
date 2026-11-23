package org.wisteria.app

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.net.VpnService
import android.webkit.JavascriptInterface
import org.json.JSONArray
import org.json.JSONObject
import org.wisteria.mobile.Mobile
import java.io.ByteArrayOutputStream
import java.net.HttpURLConnection
import java.net.URL

/**
 * The desktop App API re-implemented over the mobile engine: one
 * @JavascriptInterface entry (Call) dispatching JSON in/out. Return
 * shapes mirror the Go structs the shared frontend was written against
 * (core.Profile, core.Subscription, core.Status, core.LogLine,
 * core.WisteriaDiag). Thrown exceptions become {"__error": msg} and
 * surface as frontend toasts, like Wails rejections.
 */
class WisteriaBridge(activity: MainActivity) {

    /** The activity is held WEAKLY: connect/ping/subscription work runs on
     *  long-lived background threads, and a strong reference there outlived
     *  the destroyed activity on every rotation. appContext survives it. */
    private val activityRef = java.lang.ref.WeakReference(activity)
    private val appContext: Context = activity.applicationContext

    private val context: Context get() = appContext
    private val store by lazy { AndroidStore(context) }

    /** The live activity, or null once it is gone. */
    private fun liveActivity(): MainActivity? =
        activityRef.get()?.takeIf { !it.isFinishing && !it.isDestroyed }

    // The profile waiting for the system VPN consent dialog lives in
    // VpnConsentHolder (process-wide, survives activity recreation).
    // Cancel clears it: after a reinstall consent is revoked, so «Отмена»
    // pressed while the dialog is up must not be undone by the user then
    // granting it.

    companion object {
        /** Mirrors build.gradle's versionName so the version the settings
         *  screen shows can never disagree with the APK again (the constant
         *  here stayed "0.4.2" while the app shipped 0.4.6). */
        val VERSION: String = BuildConfig.VERSION_NAME

        /** Shared across Activity recreations so threads never leak on restartApp()/rotation. */
        private val asyncPool = java.util.concurrent.Executors.newFixedThreadPool(4) {
            Thread(it, "wisteria-bridge").apply { isDaemon = true }
        }

        /** Bounded pool for background network I/O (pings, subscription fetches, ext-IP checks),
         *  kept separate from asyncPool so slow probes never starve fast UI bindings. */
        private val ioPool = java.util.concurrent.Executors.newFixedThreadPool(4) {
            Thread(it, "wisteria-io").apply { isDaemon = true }
        }
        private const val MAX_SUB_BYTES = 4 shl 20

        /** Package-list cache lifetime. A minute keeps the Routing tab
         *  instant without hiding a just-installed app for long. */
        private const val APPS_TTL_MS = 60_000L

        /** Every "update" now ends on the public release page: Android's
         *  in-app installer flow (unknown sources + FileProvider + 90 MB
         *  download in a WebView-hosted app) was removed in favour of the
         *  browser, which keeps the APK out of the app's own trust boundary. */
        private const val RELEASES_URL = "https://github.com/qqkane/wisteria-app/releases"

        /** Status shape shared with WisteriaVpnService (EngineHub-based).
         *  startedAt (unix seconds) mirrors the desktop's Status.StartedAt:
         *  the pushed state events are what the VPN screen renders, and the
         *  uptime card reads started_at from them. */
        fun staticStatusJson(state: String, type: String, error: String? = null, startedAt: Long = 0): JSONObject =
            JSONObject()
                .put("state", state)
                .put("profile_id", EngineHub.profileId)
                .put("profile_name", EngineHub.profileName)
                .put("type", type)
                .put("started_at", if (startedAt > 0) AndroidStore.epochIso(startedAt) else "")
                .put("sys_proxy", false)
                .put("auto_profile", EngineHub.autoProfile)
                // An Android VPN is always TUN; the frontend shows its
                // "tun active" note only while this is true when connected.
                .put("tun", state == "connected")
                .put("error", error ?: JSONObject.NULL)
    }

    // ---- JS entry point ----

    @JavascriptInterface
    fun Call(name: String, argsJson: String): String {
        return try {
            val args = try { JSONArray(argsJson) } catch (_: Exception) { JSONArray() }
            val result = dispatch(name, args)
            result?.toString() ?: "null"
        } catch (e: Throwable) {
            val msg = e.message ?: e.toString()
            EngineHub.appLog("✕ $name: $msg")
            JSONObject().put("__error", msg).toString()
        }
    }

    /**
     * Asynchronous entry point. Call() runs on the WebView's JS thread, so a
     * single slow binding (ListApps enumerating 300 packages, a 15s HTTP
     * probe, PingProfile) froze the entire UI — including the Cancel button.
     * CallAsync returns immediately, runs the work on a bounded pool and
     * resolves the JS promise through window.__wisteriaResolve() on the UI
     * thread.
     */
    @JavascriptInterface
    fun CallAsync(reqId: String, name: String, argsJson: String) {
        val json = argsJson.ifBlank { "[]" }
        val task = Runnable {
            val reply = try {
                val args = try { JSONArray(json) } catch (_: Exception) { JSONArray() }
                dispatch(name, args)?.toString() ?: "null"
            } catch (t: Throwable) {
                val msg = t.message ?: t.toString()
                EngineHub.appLog("✕ $name: $msg")
                JSONObject().put("__error", msg).toString()
            }
            val act = liveActivity() ?: return@Runnable
            // reqId and reply are quoted: a JSON string literal is a valid JS
            // expression, and the shim JSON.parses the reply itself.
            act.evalJs(
                "window.__wisteriaResolve && window.__wisteriaResolve(" +
                    "${JSONObject.quote(reqId)}, ${JSONObject.quote(reply)})"
            )
        }
        try {
            asyncPool.execute(task)
        } catch (_: Throwable) {
            // Pool rejected (process shutting down): answer inline so the
            // promise still settles instead of hanging the UI forever.
            try { task.run() } catch (_: Throwable) {}
        }
    }

    private fun err(msg: String): IllegalStateException = IllegalStateException(msg)

    private fun dispatch(name: String, args: JSONArray): Any? = when (name) {
        // ---- state / logs ----
        "GetState" -> statusJson()
        "GetLogs" -> EngineHub.logsSince(args.optLong(0))
        "GetCoreHealth" -> EngineHub.lastHealth ?: JSONObject().put("active", false)
        "GetWisteriaDiag" -> diagJson()
        // JSON-encode the version string: the frontend's boot chain does
        // JSON.parse on every reply, and a bare 0.4.2 is not valid JSON.
        "Version" -> JSONObject.quote(VERSION)
        "GetExternalIP" -> externalIP()

        // ---- profiles ----
        "ListProfiles" -> store.listProfiles()
        // JSON-encode the id: a numeric-only 16-char id (the store can hand
        // one out) would otherwise arrive in JS as a NUMBER and every
        // `p.id === S.selected` comparison would fail.
        "GetActiveID" -> JSONObject.quote(store.activeId())
        "SetActive" -> { store.setActive(args.getString(0)); null }
        "Connect" -> { connect(args.getString(0)); null }
        "Disconnect" -> { disconnect(); null }
        "DeleteProfile" -> { deleteProfile(args.getString(0)); null }
        "SetProfilePreset" -> { setProfilePreset(args.getString(0), args.optString(1)); null }
        "PingProfile" -> { pingProfile(args.getString(0)); null }
        "AddLinks" -> addLinks(args.getString(0))
        "AddManual" -> addManual(args.getJSONObject(0))
        "ImportTOML" -> throw err("Импорт TOML пока только в десктоп-версии — используйте ссылку wisteria://")

        // ---- subscriptions ----
        "ListSubscriptions" -> store.listSubs()
        "AddSubscription" -> addSubscription(args.getString(0))
        "RefreshSubscription" -> refreshSubscription(args.getString(0))
        "DeleteSubscription" -> { deleteSubscription(args.getString(0)); null }
        "ImportFromClipboard" -> importFromClipboard()
        "CopyToClipboard" -> { copyToClipboard(args.optString(0)); null }

        // ---- settings ----
        "GetSettings" -> store.settings()
        "SaveSettings" -> {
            val old = store.settings()
            val next = args.getJSONObject(0)
            store.saveSettings(next)
            liveActivity()?.applyThemeFromSettings()
            maybeReconnect(old, next)
            null
        }
        // ---- vpn screen extras ----
        "GetTrafficStats" -> trafficStats()
        "TestTunnel" -> testTunnel()
        // ---- profiles power features ----
        "PingAllProfiles" -> pingAllProfiles()
        "EditProfile" -> editProfile(args.getJSONObject(0))
        "GetShareLink" -> JSONObject.quote(shareLink(args.getString(0)))
        // ---- logs / backup / system ----
        "ShareLogs" -> { shareLogs(); null }
        // Share sheet for a profile link / backup text (QR screen and edit
        // dialog reuse it instead of only copying to the clipboard).
        "ShareText" -> { shareText(args.getString(0), args.optString(1, "Wisteria")); null }
        "ExportBackup" -> { liveActivity()?.exportBackup() ?: throw err("приложение в фоне — откройте его"); null }
        "ImportBackup" -> { liveActivity()?.importBackup() ?: throw err("приложение в фоне — откройте его"); null }
        "GetBackupJson" -> JSONObject.quote(backupJson())
        "GetBatteryStatus" -> batteryStatus()
        "OpenBatterySettings" -> { openBatterySettings(); null }
        "OpenVpnSettings" -> { openVpnSettings(); null }
        "ListApps" -> listApps(args.optBoolean(0, false))
        "ListRuApps" -> listRuApps()
        // The Логи screen's «Очистить» used to clear only the DOM: the ring
        // kept growing and came back on the next tab switch.
        "ClearLogs" -> { EngineHub.clearLogs(); null }
        "GetAutostart" -> store.autostart()
        "SetAutostart" -> { store.setAutostart(args.getBoolean(0)); null }
        "GetDeviceInfo" -> JSONObject()
            .put("id", store.deviceId())
            .put("name", store.deviceName())
            .put("os", store.deviceOs())

        // ---- desktop-only, honest stubs ----
        "IsElevated" -> false
        "DetectCores" -> JSONObject()
            .put("wisteria_found", false).put("wisteria_path", "")
            .put("singbox_found", false).put("singbox_path", "")
            .put("xray_found", false).put("xray_path", "")
        "CheckAppUpdate" -> checkAppUpdate()
        "ApplyAppUpdate" -> { openReleasePage(args.optString(0)); null }
        "RestartApp" -> { liveActivity()?.restartApp(); null }
        "CheckCoreUpdate" -> JSONObject().put("current", "-").put("latest", "-").put("has_update", false)
        "UpdateCore" -> throw err("Ядра встроены в приложение (sing-box) — обновление не требуется")

        else -> throw err("неизвестный метод: $name")
    }

    // ---- state ----

    /** Engine state mapped onto core.Status (the desktop struct). Both
     *  engines report through Mobile.status(): the sing-box facade shares
     *  the same status word set as the wisteria engine. */
    fun statusJson(): JSONObject {
        // The engine library may be unavailable (failed load, stub build):
        // the UI must still render a disconnected screen instead of every
        // GetState rejecting and blanking the whole app.
        val engine = try {
            JSONObject(Mobile.status())
        } catch (t: Throwable) {
            JSONObject().put("state", "idle")
        }
        val running = isRunning(engine)
        val out = JSONObject()
        out.put("state", mapState(engine.optString("state")))
        out.put("profile_id", if (running) EngineHub.profileId else "")
        out.put("profile_name", if (running) EngineHub.profileName else "")
        out.put("type", if (running) EngineHub.profileType.ifEmpty { "wisteria" } else "")
        val since = engine.optLong("since", 0)
        out.put("started_at", if (since > 0) AndroidStore.epochIso(since) else "")
        out.put("sys_proxy", false)
        out.put("auto_profile", EngineHub.autoProfile)
        out.put("tun", running)
        if (running) {
            out.put("socks_addr", engine.optString("endpoint"))
        }
        return out
    }

    private fun isRunning(engine: JSONObject): Boolean =
        engine.optString("state") in setOf("connecting", "connected", "retrying")

    private fun mapState(s: String): String = when (s) {
        "connecting", "retrying" -> "connecting"
        "connected" -> "connected"
        else -> "disconnected"
    }

    // ---- connect / disconnect ----

    private fun connect(id: String) {
        if (id == AndroidStore.AUTO_PROFILE_ID) {
            if (store.listProfiles().length() == 0) throw err("нет профилей для автоподключения")
        } else {
            store.profile(id) ?: throw err("профиль не найден")
        }
        store.setActive(id)
        // VpnService.prepare(context) is the non-null-safe overload: it does
        // not need the activity, and the consent dialog itself is launched on
        // the UI thread by the activity when it is still alive.
        val consent = VpnService.prepare(context)
        if (consent != null) {
            VpnConsentHolder.pendingId = id
            val act = liveActivity()
            if (act != null) act.requestVpnConsent(id)
            else VpnConsentHolder.pendingId = null
            return
        }
        startConnectService(id)
    }

    private fun startConnectService(id: String) {
        val intent = Intent(context, WisteriaVpnService::class.java)
            .setAction(WisteriaVpnService.ACTION_CONNECT)
            .putExtra(WisteriaVpnService.EXTRA_PROFILE_ID, id)
        try {
            context.startForegroundService(intent)
        } catch (t: Throwable) {
            // Android 12+ refuses background FGS starts; the UI must see
            // the failure instead of sitting on the optimistic connecting
            // screen forever.
            EngineHub.appLog("запуск VPN-сервиса: ${t.message}")
            // staticStatusJson(state, type, error): the message belongs in
            // `error`, not in `type` — the old call put it in `type` and left
            // the VPN screen's red box empty.
            EngineHub.pushEvent(
                "state",
                WisteriaBridge.staticStatusJson(
                    "disconnected", "", error = "откройте приложение и повторите подключение",
                ),
            )
        }
    }

    private fun disconnect() {
        VpnConsentHolder.pendingId = null // «Отмена» beats a later grant of the dialog
        context.startService(
            Intent(context, WisteriaVpnService::class.java)
                .setAction(WisteriaVpnService.ACTION_DISCONNECT)
        )
    }

    /** Called by the activity after the VPN consent dialog resolved OK. */
    fun onVpnConsentGranted(profileId: String) {
        if (VpnConsentHolder.pendingId != profileId) { // canceled meanwhile
            VpnConsentHolder.pendingId = null
            return
        }
        VpnConsentHolder.pendingId = null
        startConnectService(profileId)
    }

    // ---- profiles ----

    private fun setProfilePreset(id: String, preset: String) {
        if (preset.isNotEmpty() && preset !in listOf("fast", "balance", "stealth")) throw err("unknown preset $preset")
        val p = store.profile(id) ?: throw err("profile not found")
        val fields = p.optJSONObject("fields") ?: JSONObject().also { p.put("fields", it) }
        if (preset.isEmpty()) fields.remove("preset") else fields.put("preset", preset)
        store.updateProfile(p)
        // The preset is baked into the engine config at connect time: a live
        // session must be rebuilt for the change to mean anything.
        if (id == EngineHub.profileId && running()) {
            EngineHub.appLog("пресет изменён — переподключение")
            reconnectService()
        }
    }

    private fun deleteProfile(id: String) {
        // running(), not a bare JSONObject(Mobile.status()): the native
        // library may be unavailable and Mobile.status() then throws straight
        // through the bridge instead of a clean refusal.
        if (id == EngineHub.profileId && running()) {
            throw err("профиль подключён — сначала отключитесь")
        }
        if (!store.deleteProfile(id)) throw err("профиль не найден")
        EngineHub.appLog("профиль удалён")
        pushProfiles()
    }

    /** Same rule as the desktop: refuse while a profile of this
     *  subscription is actually running. A merely selected profile does
     *  not block the delete (the old active-based guard did). */
    private fun deleteSubscription(id: String) {
        val running = EngineHub.profileId
        if (running.isNotEmpty() && running()) {
            val profiles = store.listProfiles()
            for (i in 0 until profiles.length()) {
                val p = profiles.getJSONObject(i)
                if (p.optString("id") == running && p.optString("sub_id") == id) {
                    throw err("подписка подключена — сначала отключитесь")
                }
            }
        }
        store.deleteSub(id)
        EngineHub.appLog("подписка удалена вместе с её профилями")
        pushProfiles()
    }

    private fun pingProfile(id: String) {
        val p = store.profile(id) ?: throw err("профиль не найден")
        // The dial itself takes seconds; on the bridge thread it would
        // freeze the whole UI (same class as the subscription fetch).
        ioPool.execute {
            val lat = try {
                if (p.optString("type") == "wisteria") {
                    Mobile.ping(endpointOf(p))
                } else {
                    tcpPing(p.optString("host"), p.optInt("port", 443))
                }
            } catch (e: Exception) {
                -1L
            }
            // Store the latency by itself: a whole-profile replace used to
            // revert edits made while the ping was in flight (preset change).
            store.updateProfileLatency(id, lat)
            pushProfiles()
        }
    }

    private fun addLinks(text: String): JSONObject {
        val added = JSONArray()
        val errors = JSONArray()
        for (rawLine in text.split("\n")) {
            val line = rawLine.trim()
            if (line.isEmpty()) continue
            if (!line.contains("://")) continue
            if (SingboxLinks.handles(line)) {
                try {
                    val p = SingboxLinks.parseProfile(line)
                    if (p == null) {
                        errors.put(JSONObject().put("line", shorten(line)).put("error", "не удалось разобрать ссылку"))
                    } else {
                        p.put("latency_ms", 0)
                        val saved = store.addProfile(p)
                        if (saved != null) added.put(saved)
                    }
                } catch (e: Exception) {
                    errors.put(JSONObject().put("line", shorten(line)).put("error", e.message ?: "не распознано"))
                }
                continue
            }
            try {
                val entries = JSONArray(Mobile.parseLinks(line))
                for (j in 0 until entries.length()) {
                    val entry = entries.getJSONObject(j)
                    if (entry.optString("key").isEmpty()) {
                        errors.put(JSONObject().put("line", shorten(line)).put("error", "приглашение без ключа: Android-версия работает по ссылкам с ключом (Ссылка клиенту в панели)"))
                        continue
                    }
                    val servers = entry.optJSONArray("servers")
                    if (servers == null || servers.length() == 0) {
                        errors.put(JSONObject().put("line", shorten(line)).put("error", "в ссылке нет серверов"))
                        continue
                    }
                    val (host, port) = splitEndpoint(servers.getJSONObject(0).optString("endpoint"))
                        ?: continue
                    val p = JSONObject()
                        .put("name", entry.optString("name").ifEmpty { host })
                        .put("type", "wisteria")
                        .put("host", host)
                        .put("port", port)
                        .put("raw", line)
                        .put("client_priv", entry.optString("key"))
                        .put("servers", servers)
                        .put("latency_ms", 0)
                    // Wave 4: the parsed rows carry the per-server TLS keys —
                    // mirror the primary row onto the profile (badge + store
                    // validation). Rows were validated by mobile.ParseLinks.
                    TlsTransport.applyPrimaryTls(p, servers)
                    val saved = store.addProfile(p)
                    if (saved != null) added.put(saved)
                }
            } catch (e: Exception) {
                errors.put(JSONObject().put("line", shorten(line)).put("error", e.message ?: "не распознано"))
            }
        }
        if (added.length() > 0) {
            EngineHub.appLog("импорт: профилей ${added.length()}")
            pushProfiles()
        }
        return JSONObject().put("added", added).put("errors", errors)
    }

    private fun addManual(o: JSONObject): JSONObject {
        val type = o.optString("type")
        if (type != "wisteria") {
            throw err("тип «$type» пока работает только в десктоп-версии (Android: wisteria)")
        }
        val host = o.optString("host").trim()
        val port = o.optInt("port", 0)
        if (host.isEmpty() || port <= 0) throw err("укажите адрес и порт сервера")
        // Wave 4 TLS transport: "tcp" from the form is normalized onto the
        // absent default by the store choke point; here it only decides the
        // dedup identity — a bare-tcp and a TLS profile on the same
        // host:port are different endpoints and may coexist.
        val tls = o.optString("transport").trim() == "tls"
        // Manual profiles carry no raw link, so addProfile's raw-dedup
        // never fires for them — compare by type+host+port explicitly.
        val existing = store.listProfiles()
        for (i in 0 until existing.length()) {
            val p = existing.getJSONObject(i)
            if (p.optString("type") == "wisteria" &&
                p.optString("host") == host && p.optInt("port", 0) == port &&
                (p.optString("transport") == "tls") == tls
            ) {
                throw err("такой профиль уже есть")
            }
        }
        val fields = o.optJSONObject("fields") ?: JSONObject()
        val priv = fields.optString("priv")
        val keypair = if (priv.isEmpty()) {
            JSONObject(Mobile.generateKeypair())
        } else {
            JSONObject().put("priv", priv).put("pub", Mobile.myPublicKey(priv))
        }
        val p = JSONObject()
            .put("name", o.optString("name").ifEmpty { host })
            .put("type", "wisteria")
            .put("host", host)
            .put("port", port)
            .put("client_priv", keypair.optString("priv"))
            .put("latency_ms", 0)
        fields.optString("pub").takeIf { it.isNotEmpty() }?.let { p.put("pub", it) }
        // Wisteria obfuscation knobs from the manual form. They ride the
        // profile's `fields` object (same shape the desktop saves and
        // WisteriaVpnService.presetOf() reads), otherwise the manual form
        // silently dropped every value the user typed.
        val obs = JSONObject()
        for (key in listOf("min_pad", "max_pad", "mimic", "obf_mu", "obf_sigma")) {
            val v = fields.optString(key).trim()
            if (v.isNotEmpty()) obs.put(key, v)
        }
        fields.optJSONObject("obs")?.let { src ->
            for (key in listOf("min_pad", "max_pad", "mimic", "obf_mu", "obf_sigma")) {
                if (!obs.has(key)) src.optString(key).takeIf { it.isNotEmpty() }?.let { obs.put(key, it) }
            }
        }
        if (obs.length() > 0) p.put("fields", obs)
        // TLS fields ride the payload top-level (the select always sends
        // "tcp"/"tls"; the SNI and the checkbox only matter with TLS).
        // Validation+canonicalization happen in addProfile → normalizeTls:
        // a bad SNI or sni/insecure-without-tls comes back as an error.
        o.optString("transport").takeIf { it.isNotEmpty() }?.let { p.put("transport", it) }
        o.optString("tls_sni").takeIf { it.isNotEmpty() }?.let { p.put("tls_sni", it.trim()) }
        if (o.optBoolean("tls_insecure")) p.put("tls_insecure", true)
        val saved = store.addProfile(p) ?: throw err("такой профиль уже есть")
        EngineHub.appLog("профиль «${saved.optString("name")}» добавлен вручную")
        pushProfiles()
        return saved
    }

    // ---- subscriptions ----

    private fun addSubscription(url: String): JSONObject {
        val sub = store.addSub(url.trim()) ?: throw err("такая подписка уже есть")
        EngineHub.appLog("подписка добавлена: ${sub.optString("name")}")
        fetchSubAsync(sub)
        return sub
    }

    private fun refreshSubscription(id: String): JSONObject {
        val sub = store.sub(id) ?: throw err("подписка не найдена")
        fetchSubAsync(sub)
        return sub
    }

    /** The fetch (up to 15s of blocking HTTP) must never run on the bridge
     *  thread: a @JavascriptInterface call holds the WebView's JS thread,
     *  so the whole UI — including the Cancel button — freezes meanwhile.
     *  Runs in its own thread and reports through the "profiles" event. */
    private fun fetchSubAsync(sub: JSONObject) {
        ioPool.execute {
            try {
                fetchIntoSub(sub)
            } catch (t: Throwable) {
                // Throwable, not Exception: an Error (OOM on a huge body, an
                // android.util.AndroidRuntimeException from a stub) on this
                // bare thread would kill the whole process.
                val msg = t.message ?: t.javaClass.simpleName
                try {
                    sub.put("error", msg)
                    store.updateSub(sub)
                } catch (_: Throwable) {
                }
                EngineHub.appLog("подписка «${sub.optString("name")}»: $msg")
            }
            pushProfiles()
        }
    }

    /** Fetches the subscription body, parses every link (wisteria and
     *  sing-box protocols) and replaces exactly this subscription's
     *  profiles, keeping the client keys of endpoints seen before (the
     *  desktop merge rule). */
    private fun fetchIntoSub(sub: JSONObject): JSONObject {
        val res = httpGetWithHeaders(sub.optString("url"), MAX_SUB_BYTES, 15_000)
        val text = decodeSubBody(res.body)
        if (SubscriptionMeta.looksLikeHtml(text)) {
            throw err("сервер вернул HTML-страницу вместо списка ссылок — подписка не изменена")
        }

        // sing-box protocol lines never reach Mobile.parseLinks; pull them
        // out first and hand Mobile only the rest (wisteria invites).
        val sbProfiles = mutableListOf<JSONObject>()
        val rest = StringBuilder()
        for (rawLine in text.split("\n")) {
            val line = rawLine.trim()
            if (line.isNotEmpty() && SingboxLinks.handles(line)) {
                try {
                    SingboxLinks.parseProfile(line)?.let { sbProfiles.add(it) }
                } catch (_: Exception) {
                }
            } else {
                rest.append(rawLine).append("\n")
            }
        }
        // A sub with no wisteria lines (or plain junk in the mix) must not
        // lose the sing-box profiles extracted above.
        val entries = try {
            if (rest.isNotBlank()) JSONArray(Mobile.parseLinks(rest.toString())) else JSONArray()
        } catch (e: Exception) {
            EngineHub.appLog("подписка: ссылки wisteria не разобраны (${e.message})")
            JSONArray()
        }

        // Nothing parseable at all: the body is junk (or every line was an
        // invite without a key). Deleting the old profiles now would leave
        // the user with an empty subscription and no way back.
        if (sbProfiles.isEmpty() && entries.length() == 0) {
            throw err("в подписке не найдено ни одной рабочей ссылки — профили сохранены")
        }

        // Everything parsed: apply the response metadata before touching the
        // profiles, so the row shows the real name/counters even if the merge
        // below hits the store's dedup.
        SubscriptionMeta.apply(sub, res.headers)

        val old = JSONArray()
        val profiles = store.listProfiles()
        for (i in 0 until profiles.length()) {
            val p = profiles.getJSONObject(i)
            if (p.optString("sub_id") == sub.optString("id")) old.put(p)
        }
        val keyByEndpoint = HashMap<String, String>()
        for (i in 0 until old.length()) {
            val p = old.getJSONObject(i)
            if (p.optString("client_priv").isNotEmpty()) {
                keyByEndpoint[p.optString("host") + ":" + p.optInt("port")] = p.optString("client_priv")
            }
        }
        // The active profile survives the wipe below (deleting a running
        // profile would orphan the live engine). Re-adding its endpoint
        // from the fresh body would duplicate it (wisteria entries carry
        // no raw link, so the store's raw-dedup never fires), so the
        // matching entry is skipped instead.
        val activeId = store.activeId()
        var keptActiveEndpoint: String? = null
        var keptActiveBoxKey: String? = null
        for (i in 0 until old.length()) {
            val p = old.getJSONObject(i)
            if (p.optString("id") == activeId && activeId.isNotEmpty()) {
                keptActiveEndpoint = p.optString("host") + ":" + p.optInt("port")
                // sing-box profiles: dedup by endpoint+type as well, or a
                // refreshed link with changed parameters (new uuid/password,
                // same server) added a second row next to the running one.
                keptActiveBoxKey = p.optString("type") + "|" + keptActiveEndpoint
                break
            }
        }

        for (i in 0 until old.length()) {
            val p = old.getJSONObject(i)
            if (p.optString("id") != activeId) store.deleteProfile(p.optString("id"))
        }

        // The preserved active profile is part of the subscription: count it,
        // otherwise the row says "3 профиля" next to 4 visible rows.
        var count = if (keptActiveEndpoint != null) 1 else 0
        for (j in 0 until entries.length()) {
            val entry = entries.getJSONObject(j)
            if (entry.optString("key").isEmpty()) continue // invites: unusable on Android
            val servers = entry.optJSONArray("servers") ?: continue
            if (servers.length() == 0) continue
            val (host, port) = splitEndpoint(servers.getJSONObject(0).optString("endpoint")) ?: continue
            if (keptActiveEndpoint != null && keptActiveEndpoint == host + ":" + port) continue
            val p = JSONObject()
                .put("name", entry.optString("name").ifEmpty { host })
                .put("type", "wisteria")
                .put("host", host)
                .put("port", port)
                .put("raw", "")
                .put("sub_id", sub.optString("id"))
                .put("client_priv", keyByEndpoint[host + ":" + port] ?: entry.optString("key"))
                .put("servers", servers)
                .put("latency_ms", 0)
            // Wave 4: mirror the primary row's TLS keys onto the profile
            // (same rule as addLinks; rows were validated by ParseLinks).
            TlsTransport.applyPrimaryTls(p, servers)
            store.addProfile(p)?.let { count++ }
        }
        for (p in sbProfiles) {
            p.put("sub_id", sub.optString("id")).put("latency_ms", 0)
            val ep = p.optString("host") + ":" + p.optInt("port")
            if (keptActiveBoxKey != null && keptActiveBoxKey == p.optString("type") + "|" + ep) continue
            store.addProfile(p)?.let { count++ }
        }
        sub.put("count", count).put("updated_at", AndroidStore.nowIso()).put("error", "")
        store.updateSub(sub)
        EngineHub.appLog("подписка «${sub.optString("name")}» обновлена: профилей $count")
        pushProfiles()
        return sub
    }

    private fun importFromClipboard(): JSONObject {
        val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as? ClipboardManager
        val text = cm?.primaryClip?.getItemAt(0)?.text?.toString() ?: ""
        if (text.isBlank()) throw err("буфер обмена пуст")

        var subCount = 0
        val links = StringBuilder()
        for (rawLine in text.split("\n")) {
            val line = rawLine.trim()
            if (line.isEmpty()) continue
            if (line.startsWith("http://") || line.startsWith("https://")) {
                try {
                    val sub = store.addSub(line)
                    if (sub != null) {
                        subCount++
                        fetchSubAsync(sub) // fetch off the bridge thread
                    }
                } catch (e: Exception) {
                    EngineHub.appLog("подписка ${maskUrl(line)}: ${e.message}")
                }
            } else {
                links.append(line).append("\n")
            }
        }
        val res = if (links.isNotEmpty()) addLinks(links.toString())
        else JSONObject().put("added", JSONArray()).put("errors", JSONArray())
        return res.put("subscriptions", subCount)
    }

    // Clipboard writes must happen on the UI thread: @JavascriptInterface calls
    // arrive on a private WebView thread, and Android 10+ only honours the write
    // from the focused activity.
    private fun copyToClipboard(text: String) {
        if (text.isEmpty()) return
        val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as? ClipboardManager ?: return
        val clip = ClipData.newPlainText("Wisteria logs", text)
        android.os.Handler(android.os.Looper.getMainLooper()).post {
            try {
                cm.setPrimaryClip(clip)
            } catch (_: Throwable) {
            }
        }
    }

    // ---- external IP / diag ----

    // A @JavascriptInterface call is synchronous for the WebView JS thread:
    // a slow reply freezes the whole UI (this is what "приложение зависло"
    // was — a 10s HTTP probe through a dead tunnel on every vpn render).
    // The check therefore always runs on its own thread and reports through
    // the "extip" event; the bridge call itself returns immediately.
    private val extIpChecking = java.util.concurrent.atomic.AtomicBoolean(false)

    private fun requestExtIP() {
        if (!extIpChecking.compareAndSet(false, true)) return
        ioPool.execute {
            try {
                // Mirrors in turn: a single slow or throttled checker (ipify
                // in particular) used to leave the IP card empty forever.
                val mirrors = listOf(
                    "https://api.ipify.org",
                    "https://ipinfo.io/ip",
                    "https://ifconfig.me/ip",
                )
                var ip = ""
                var lastError: Exception? = null
                for (m in mirrors) {
                    try {
                        ip = httpGet(m, 1024, 10_000).trim()
                        if (ip.isNotEmpty()) break
                    } catch (e: Exception) {
                        lastError = e
                    }
                }
                if (ip.isNotEmpty()) EngineHub.pushEvent("extip", JSONObject().put("ip", ip))
                else EngineHub.pushEvent("extip", JSONObject().put("error", lastError?.message ?: "пустой ответ"))
            } catch (e: Exception) {
                EngineHub.pushEvent("extip", JSONObject().put("error", e.message ?: ""))
            } finally {
                extIpChecking.set(false)
            }
        }
    }

    private fun externalIP(): String {
        if (!isRunning(JSONObject(Mobile.status()))) throw err("нет активного подключения")
        // split_mode "only" means Wisteria itself is NOT in the tunnel (the
        // app is never added to addAllowedApplication). Any IP probe we make
        // therefore reports the real ISP address while the UI calls it
        // "protected" — say so instead of lying.
        if (store.settings().optString("split_mode") == "only") {
            EngineHub.pushEvent(
                "extip",
                JSONObject()
                    .put("mode_only", true)
                    .put("error", "режим «только выбранные»: приложение вне туннеля, внешний IP — реальный")
            )
            return ""
        }
        requestExtIP()
        return "" // the result arrives via the "extip" event
    }

    fun onConnectedCheckIP() {
        val s = store.settings()
        if (!s.optBoolean("check_ext_ip", true)) return
        // In "only" mode the app itself is outside the tunnel: the probe
        // would report the real ISP address as if it were the exit IP.
        if (s.optString("split_mode") == "only") return
        requestExtIP()
    }

    private fun diagJson(): JSONObject {
        val st = statusJson()
        val active = st.optString("state") != "disconnected"
        val out = JSONObject()
            .put("active", active)
            .put("status", st)
            .put("servers", diagServersJson(st))
            .put("updated", System.nanoTime())
        val events = EngineHub.diagEventsJson()
        out.put("events", events)
        var lastKind = ""
        if (events.length() > 0) lastKind = events.getJSONObject(events.length() - 1).optString("kind")
        out.put("last_kind", lastKind)
        return out
    }

    /** The diagnostics page chips (best RTT / reachable servers) are computed
     *  from the WServerStat list the desktop core fills in; the mobile engine
     *  has no such report, so synthesize it from the active profile: one row
     *  per configured endpoint. The engine reports the endpoint it is
     *  ACTUALLY connected to in status.endpoint (socks_addr here) — with a
     *  multi-server profile that is not servers[0], and the old code marked
     *  servers[0] reachable no matter which mirror carried the session. */
    private fun diagServersJson(st: JSONObject): JSONArray {
        if (st.optString("state") == "disconnected") return JSONArray()
        val prof = store.profile(EngineHub.profileId) ?: return JSONArray()
        val connected = st.optString("state") == "connected"
        val activeEndpoint = st.optString("socks_addr").trim()
        val primary = endpointOf(prof)
        val latency = prof.optLong("latency_ms", 0)
        val endpoints = LinkedHashSet<String>()
        prof.optJSONArray("servers")?.let { arr ->
            for (i in 0 until arr.length()) {
                val ep = arr.getJSONObject(i).optString("endpoint")
                if (ep.isNotEmpty()) endpoints.add(ep)
            }
        }
        if (primary.isNotEmpty()) endpoints.add(primary) // sing-box profiles (no servers array) land here
        if (activeEndpoint.isNotEmpty()) endpoints.add(activeEndpoint)
        val out = JSONArray()
        for (ep in endpoints) {
            val isActive = activeEndpoint.isNotEmpty() && ep == activeEndpoint
            val isPrimary = ep == primary
            out.put(
                JSONObject()
                    .put("endpoint", ep)
                    .put("rtt_ms", if ((isActive || isPrimary) && latency > 0) latency else 0)
                    .put("reachable", connected && (isActive || (activeEndpoint.isEmpty() && isPrimary)))
                    .put("active", isActive)
                    .put("failures", if (isActive || isPrimary) EngineHub.retryEvents else 0)
            )
        }
        return out
    }

    // ---- helpers ----

    // ---- live traffic ----
    // TrafficStats counters are process-wide and monotonic; the bridge keeps
    // the baseline of the current session and the previous sample, so the UI
    // can show "down/up now" plus "transferred this session". Everything
    // degrades to zeros when the OS reports UNSUPPORTED (some emulators and
    // vendor ROMs do).
    private var trafficSessionKey = ""
    private var trafficBaseRx = -1L
    private var trafficBaseTx = -1L
    private var trafficLastRx = -1L
    private var trafficLastTx = -1L
    private var trafficLastAt = 0L

    private fun rxBytes(): Long {
        val uid = try {
            android.net.TrafficStats.getUidRxBytes(android.os.Process.myUid())
        } catch (_: Throwable) {
            android.net.TrafficStats.UNSUPPORTED.toLong()
        }
        if (uid >= 0) return uid
        val total = try {
            android.net.TrafficStats.getTotalRxBytes()
        } catch (_: Throwable) {
            -1L
        }
        return if (total >= 0) total else -1L
    }

    private fun txBytes(): Long {
        val uid = try {
            android.net.TrafficStats.getUidTxBytes(android.os.Process.myUid())
        } catch (_: Throwable) {
            android.net.TrafficStats.UNSUPPORTED.toLong()
        }
        if (uid >= 0) return uid
        val total = try {
            android.net.TrafficStats.getTotalTxBytes()
        } catch (_: Throwable) {
            -1L
        }
        return if (total >= 0) total else -1L
    }

    /** {rx_speed, tx_speed (bytes/s), rx_total, tx_total (bytes), ok} */
    private fun trafficStats(): JSONObject {
        val now = System.currentTimeMillis()
        val st = try { JSONObject(Mobile.status()) } catch (_: Throwable) { JSONObject() }
        val running = isRunning(st)
        val out = JSONObject()
        if (!running) {
            trafficSessionKey = ""
            trafficBaseRx = -1L
            trafficBaseTx = -1L
            trafficLastRx = -1L
            trafficLastTx = -1L
            trafficLastAt = 0L
            return out.put("ok", false)
                .put("rx_speed", 0).put("tx_speed", 0)
                .put("rx_total", 0).put("tx_total", 0)
        }
        // A new engine session invalidates every counter baseline. The
        // mobile status has no monotonic "since" counter; started_at is the
        // per-session wall clock, so a reconnect with the same profile still
        // starts a fresh total.
        val key = EngineHub.profileId + "@" + st.optString("started_at", st.optString("since", ""))
        val rx = rxBytes()
        val tx = txBytes()
        if (key != trafficSessionKey) {
            trafficSessionKey = key
            trafficBaseRx = rx
            trafficBaseTx = tx
            trafficLastRx = rx
            trafficLastTx = tx
            trafficLastAt = now
        }
        val dt = if (trafficLastAt > 0) now - trafficLastAt else 0
        val down = if (rx >= 0 && trafficLastRx >= 0 && dt > 0) {
            ((rx - trafficLastRx).coerceAtLeast(0) * 1000 / dt)
        } else 0
        val up = if (tx >= 0 && trafficLastTx >= 0 && dt > 0) {
            ((tx - trafficLastTx).coerceAtLeast(0) * 1000 / dt)
        } else 0
        trafficLastRx = rx
        trafficLastTx = tx
        trafficLastAt = now
        return out.put("ok", rx >= 0)
            .put("rx_speed", down)
            .put("tx_speed", up)
            .put("rx_total", if (rx >= 0 && trafficBaseRx >= 0) (rx - trafficBaseRx).coerceAtLeast(0) else 0)
            .put("tx_total", if (tx >= 0 && trafficBaseTx >= 0) (tx - trafficBaseTx).coerceAtLeast(0) else 0)
    }

    /** Real end-to-end check through the tunnel: a 204 from gstatic proves
     *  DNS, TCP and TLS all work over the established session. */
    private fun testTunnel(): JSONObject {
        val started = System.nanoTime()
        return try {
            val conn = URL("https://www.gstatic.com/generate_204").openConnection() as HttpURLConnection
            conn.connectTimeout = 5000
            conn.readTimeout = 5000
            conn.instanceFollowRedirects = true
            conn.setRequestProperty("User-Agent", "WisteriaAndroid/$VERSION")
            try {
                val code = conn.responseCode
                val ms = (System.nanoTime() - started) / 1_000_000
                if (code in 200..399) {
                    JSONObject().put("ok", true).put("rtt_ms", ms).put("status", code)
                } else {
                    JSONObject().put("ok", false).put("error", "HTTP $code")
                }
            } finally {
                conn.disconnect()
            }
        } catch (e: Throwable) {
            JSONObject().put("ok", false).put("error", e.message ?: e.javaClass.simpleName)
        }
    }

    // ---- ping all / edit / share ----

    private fun pingOne(p: JSONObject): Long = try {
        if (p.optString("type") == "wisteria") {
            Mobile.ping(endpointOf(p))
        } else {
            tcpPing(p.optString("host"), p.optInt("port", 443))
        }
    } catch (_: Throwable) {
        -1L
    }

    /** Every profile pinged with a bounded pool (6 workers): a sequential
     *  pass over 30 profiles is a minute of dead UI, an unbounded one hammers
     *  the radio and the servers. */
    private fun pingAllProfiles(): JSONArray {
        val list = store.listProfiles()
        val n = list.length()
        val results = java.util.Collections.synchronizedList(ArrayList<JSONObject>(n))
        val workers = maxOf(1, minOf(6, n))
        val pool = java.util.concurrent.Executors.newFixedThreadPool(workers) { r ->
            Thread(r, "wisteria-ping").apply { isDaemon = true }
        }
        try {
            val futures = ArrayList<java.util.concurrent.Future<*>>(n)
            for (i in 0 until n) {
                val p = list.getJSONObject(i)
                val id = p.optString("id")
                futures.add(pool.submit {
                    val lat = pingOne(p)
                    store.updateProfileLatency(id, lat)
                    results.add(JSONObject().put("id", id).put("latency_ms", lat))
                })
            }
            for (f in futures) {
                try {
                    f.get()
                } catch (_: Throwable) {
                }
            }
        } finally {
            pool.shutdown()
        }
        pushProfiles()
        val out = JSONArray()
        for (r in results) out.put(r)
        return out
    }

    /** Rename + endpoint + wisteria TLS fields. Only the keys present in the
     *  payload are touched, so an old UI cannot erase a field it does not
     *  know about. */
    private fun editProfile(o: JSONObject): JSONObject {
        val id = o.optString("id")
        val p = store.profile(id) ?: throw err("профиль не найден")
        val name = o.optString("name").trim()
        if (name.isNotEmpty()) p.put("name", name)
        val host = o.optString("host").trim()
        if (host.isNotEmpty()) p.put("host", host)
        val port = o.optInt("port", 0)
        if (port in 1..65535) p.put("port", port)
        // A bundle profile stores the whole mirror list and the engine dials
        // THAT, not host/port: without rewriting servers[0] an edit was a
        // no-op for the live engine (and for the row the card displays).
        val servers = p.optJSONArray("servers")
        if (servers != null && servers.length() > 0) {
            val row = servers.getJSONObject(0)
            val h = if (host.isNotEmpty()) host else p.optString("host")
            val prt = if (port in 1..65535) port else p.optInt("port", 0)
            row.put("endpoint", if (prt > 0) "$h:$prt" else h)
            if (o.has("transport")) {
                if (o.optString("transport") == "tls") row.put("transport", "tls") else row.remove("transport")
            }
            if (o.has("tls_sni")) {
                val sni = o.optString("tls_sni").trim()
                if (sni.isEmpty()) row.remove("tls_sni") else row.put("tls_sni", sni)
            }
            if (o.has("tls_insecure")) {
                if (o.optBoolean("tls_insecure")) row.put("tls_insecure", true) else row.remove("tls_insecure")
            }
            p.put("servers", servers)
            // The profile-level TLS keys mirror the primary row (see
            // applyPrimaryTls); rebuild them so the badge and the store
            // validation match what the engine will dial.
            p.remove("transport")
            p.remove("tls_sni")
            p.remove("tls_insecure")
            TlsTransport.applyPrimaryTls(p, servers)
        } else {
            if (o.has("transport")) p.put("transport", o.optString("transport"))
            if (o.has("tls_sni")) p.put("tls_sni", o.optString("tls_sni").trim())
            if (o.has("tls_insecure")) p.put("tls_insecure", o.optBoolean("tls_insecure"))
        }
        // Same validation choke point as every import path.
        TlsTransport.normalizeTls(p)
        store.updateProfile(p)
        EngineHub.appLog("профиль «${p.optString("name")}» изменён")
        // A live session keeps the config it started with: rebuild it so the
        // edit is not a no-op until the next manual reconnect.
        if (id == EngineHub.profileId && running()) {
            EngineHub.appLog("профиль активен — переподключение")
            reconnectService()
        }
        pushProfiles()
        return p
    }

    /** The original share link. Profiles imported from a link keep it in
     *  `raw`; manual and subscription profiles have none (a wisteria link
     *  carries the client key, which is why the store keeps raw instead of
     *  reconstructing it). */
    private fun shareLink(id: String): String {
        val p = store.profile(id) ?: throw err("профиль не найден")
        val raw = p.optString("raw")
        if (raw.isNotEmpty()) return raw
        throw err("у этого профиля нет share-ссылки — он добавлен вручную или из подписки")
    }

    // ---- logs sharing / backup ----

    private fun shareText(text: String, subject: String) {
        if (text.isEmpty()) throw err("нечего отправлять")
        val send = Intent(Intent.ACTION_SEND).apply {
            type = "text/plain"
            putExtra(Intent.EXTRA_SUBJECT, subject)
            putExtra(Intent.EXTRA_TEXT, text)
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        try {
            context.startActivity(Intent.createChooser(send, subject).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        } catch (t: Throwable) {
            throw err("некому поделиться: ${t.message}")
        }
    }

    private fun shareLogs(): String {
        val lines = EngineHub.logsSince(0)
        val sb = StringBuilder()
        sb.append("Wisteria ").append(VERSION).append(" · state=")
            .append(try { JSONObject(Mobile.status()).optString("state") } catch (_: Throwable) { "?" })
            .append("\n--- logs ---\n")
        for (i in 0 until lines.length()) {
            val l = lines.getJSONObject(i)
            sb.append(l.optString("at")).append(" [").append(l.optString("tag")).append("] ")
                .append(l.optString("text")).append('\n')
        }
        val text = sb.toString()
        val send = Intent(Intent.ACTION_SEND).apply {
            type = "text/plain"
            putExtra(Intent.EXTRA_SUBJECT, "Wisteria logs")
            putExtra(Intent.EXTRA_TEXT, text)
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        try {
            context.startActivity(Intent.createChooser(send, "Wisteria logs").addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        } catch (t: Throwable) {
            throw err("некому поделиться: ${t.message}")
        }
        return text
    }

    /** Everything worth restoring: profiles, subscriptions, settings. The
     *  client keys live inside the profiles, so this file IS a credential —
     *  the UI says as much. */
    internal fun backupJson(): String {
        val out = JSONObject()
            .put("format", "wisteria-android-backup")
            .put("version", 1)
            .put("exported_at", AndroidStore.nowIso())
            .put("app_version", VERSION)
            .put("profiles", store.listProfiles())
            .put("subs", store.listSubs())
            .put("settings", store.settings())
            .put("active", store.activeId())
            .put("autostart", store.autostart())
        return out.toString(2)
    }

    /** Restores a backup produced by backupJson(). Field names match the
     *  store's own JSON, so old data keeps working. Returns a summary. */
    internal fun restoreBackup(text: String): JSONObject {
        val o = try {
            JSONObject(text)
        } catch (e: Exception) {
            throw err("это не файл резервной копии Wisteria")
        }
        if (o.optString("format") != "wisteria-android-backup") {
            throw err("это не файл резервной копии Wisteria")
        }
        val profiles = o.optJSONArray("profiles") ?: JSONArray()
        val subs = o.optJSONArray("subs") ?: JSONArray()
        var pAdd = 0
        var sAdd = 0
        // Profiles reference their subscription by id. addSub() mints a new
        // id (and returns null for a URL that is already stored), so the
        // backup's ids must be remapped — otherwise every restored profile
        // pointed at a sub row that does not exist and landed in "manual".
        val subIdMap = HashMap<String, String>()
        for (i in 0 until subs.length()) {
            val s = subs.getJSONObject(i)
            val url = s.optString("url")
            if (url.isEmpty()) continue
            val oldId = s.optString("id")
            val created = store.addSub(url)
            if (created == null) {
                // Already stored: reuse the existing row's id.
                val all = store.listSubs()
                for (j in 0 until all.length()) {
                    val ex = all.getJSONObject(j)
                    if (ex.optString("url") == url) {
                        if (oldId.isNotEmpty()) subIdMap[oldId] = ex.optString("id")
                        break
                    }
                }
                continue
            }
            s.put("id", created.optString("id"))
            if (oldId.isNotEmpty()) subIdMap[oldId] = created.optString("id")
            // Keep the backup's name/counters on the freshly created row.
            store.updateSub(s)
            sAdd++
        }
        for (i in 0 until profiles.length()) {
            val p = profiles.getJSONObject(i)
            if (p.optString("id").isEmpty()) continue
            if (store.profile(p.optString("id")) != null) continue
            subIdMap[p.optString("sub_id")]?.let { p.put("sub_id", it) }
            try {
                if (store.addProfile(p) != null) pAdd++
            } catch (_: Throwable) {
                // A profile whose TLS combo no longer validates is skipped,
                // not fatal: the rest of the backup still restores.
            }
        }
        if (o.has("settings")) {
            val cur = store.settings()
            val src = o.optJSONObject("settings") ?: JSONObject()
            // Only known keys: an old backup must not inject new ones.
            src.keys().forEach { k -> if (cur.has(k)) cur.put(k, src.opt(k)) }
            store.saveSettings(cur)
        }
        val active = o.optString("active")
        if (active.isNotEmpty() && store.profile(active) != null) store.setActive(active)
        if (o.has("autostart")) store.setAutostart(o.optBoolean("autostart"))
        EngineHub.appLog("восстановление: профилей $pAdd, подписок $sAdd")
        pushProfiles()
        return JSONObject().put("profiles", pAdd).put("subs", sAdd)
    }

    // ---- battery / system settings ----

    private fun batteryStatus(): JSONObject {
        val pm = context.getSystemService(Context.POWER_SERVICE) as? android.os.PowerManager
        val ignoring = try {
            pm?.isIgnoringBatteryOptimizations(context.packageName) ?: false
        } catch (_: Throwable) {
            false
        }
        return JSONObject().put("ignoring", ignoring)
    }

    private fun openBatterySettings() {
        val intent = Intent(android.provider.Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        // Some ROMs rename/remove the list; the per-app request dialog is the
        // fallback (it needs the REQUEST_IGNORE_BATTERY_OPTIMIZATIONS
        // permission, which the manifest declares).
        try {
            context.startActivity(intent)
        } catch (_: Throwable) {
            try {
                context.startActivity(
                    Intent(android.provider.Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS)
                        .setData(Uri.parse("package:${context.packageName}"))
                        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                )
            } catch (t: Throwable) {
                throw err("настройки батареи недоступны: ${t.message}")
            }
        }
    }

    private fun openVpnSettings() {
        try {
            context.startActivity(
                Intent(android.provider.Settings.ACTION_VPN_SETTINGS).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            )
        } catch (t: Throwable) {
            throw err("настройки VPN недоступны: ${t.message}")
        }
    }

    private fun pushProfiles() {
        EngineHub.pushEvent("profiles", null)
    }

    /** TCP connect RTT for sing-box-protocol profiles (no engine ping). */
    private fun tcpPing(host: String, port: Int): Long {
        val start = System.nanoTime()
        java.net.Socket().use { s ->
            s.connect(java.net.InetSocketAddress(host, if (port in 1..65535) port else 443), 3000)
        }
        return (System.nanoTime() - start) / 1_000_000
    }

    fun endpointOf(p: JSONObject): String {
        val host = p.optString("host")
        val port = p.optInt("port", 0)
        return if (port > 0) "$host:$port" else host
    }

    /** Subscription URLs commonly carry an auth token in the query; keep
     *  it out of the log ring the Логи screen shows. */
    private fun maskUrl(u: String): String = u.substringBefore('?').substringBefore('#')

    /** Import-error lines are shown verbatim in the errors box; a full
     *  wisteria:// line would carry the whole client key there. The
     *  desktop caps at the same 42 chars. */
    private fun shorten(s: String): String =
        if (s.length <= 42) s else s.take(42) + "…"

    /** host:port → (host, port); null when the endpoint is unusable. */
    private fun splitEndpoint(ep: String): Pair<String, Int>? {
        val idx = ep.lastIndexOf(':')
        if (idx <= 0 || idx == ep.length - 1) return null
        val port = ep.substring(idx + 1).toIntOrNull() ?: return null
        if (port <= 0 || port > 65535) return null
        val host = ep.substring(0, idx).removePrefix("[").removeSuffix("]")
        if (host.isEmpty()) return null
        return host to port
    }

    // ---- split tunnel / live reconnect ----

    /** Installed apps for the per-app split-tunnel picker: launchable apps
     *  plus user-installed ones, minus ourselves. The manifest <queries>
     *  (MAIN/LAUNCHER) makes them visible under Android 11+ package rules.
     *  Cached: enumerating ~300 packages is the single slowest binding the
     *  Routing tab touches, and the list only changes on install/uninstall. */
    @Volatile
    private var appsCache: JSONArray? = null

    @Volatile
    private var appsCacheAt = 0L

    private fun listApps(force: Boolean = false): JSONArray {
        val now = System.currentTimeMillis()
        appsCache?.let { if (!force && now - appsCacheAt < APPS_TTL_MS) return it }
        val pm = context.packageManager
        val rows = ArrayList<Pair<String, String>>()
        for (ai in pm.getInstalledApplications(0)) {
            if (ai.packageName == context.packageName) continue
            val launchable = pm.getLaunchIntentForPackage(ai.packageName) != null
            val userApp = (ai.flags and android.content.pm.ApplicationInfo.FLAG_SYSTEM) == 0
            if (!launchable && !userApp) continue
            rows.add(ai.packageName to pm.getApplicationLabel(ai).toString())
        }
        rows.sortBy { it.second.lowercase() }
        val out = JSONArray()
        for ((pkg, label) in rows) out.put(JSONObject().put("pkg", pkg).put("label", label))
        appsCache = out
        appsCacheAt = now
        return out
    }

    private fun running(): Boolean = try {
        isRunning(JSONObject(Mobile.status()))
    } catch (_: Throwable) {
        false
    }

    /** The RU bypass preset: only packages present on this device. */
    private fun listRuApps(): JSONArray {
        val out = JSONArray()
        for ((pkg, label) in RuApps.installed(context)) {
            out.put(JSONObject().put("pkg", pkg).put("label", label))
        }
        return out
    }

    /** Effective preset of the running profile: per-profile wins over global. */
    private fun effPreset(s: JSONObject): String {
        val per = store.profile(EngineHub.profileId)
            ?.optJSONObject("fields")?.optString("preset").orEmpty()
        val v = per.ifEmpty { s.optString("wisteria_preset", "stealth") }
        return when (v) { "fast", "balance", "stealth" -> v; else -> "stealth" }
    }

    /** Tunnel-shaping settings only apply to a fresh VpnService.Builder:
     *  when any of them changes under a live session, rebuild it. */
    private fun maybeReconnect(old: JSONObject, next: JSONObject) {
        if (!running()) return
        val splitChanged =
            old.optString("split_mode", "off") != next.optString("split_mode", "off") ||
                (old.optJSONArray("split_apps")?.toString() ?: "[]") !=
                (next.optJSONArray("split_apps")?.toString() ?: "[]")
        val routesChanged =
            old.optBoolean("no_bypass_ru", false) != next.optBoolean("no_bypass_ru", false) ||
                old.optString("wisteria_direct_cidrs") != next.optString("wisteria_direct_cidrs")
        val presetChanged = effPreset(old) != effPreset(next)
        // sing-box bakes its whole routing/DNS/tun block into the config it
        // was started with: changing any of these used to take effect only
        // after a manual reconnect, which read as "the toggle does nothing".
        val boxSettings = listOf(
            "bypass_lan", "route_direct", "route_block", "remote_dns",
            "log_level", "tun_stack", "tun_strict_route",
        )
        val boxChanged = EngineHub.profileType != "wisteria" && EngineHub.profileType.isNotEmpty() &&
            boxSettings.any { old.optString(it) != next.optString(it) }
        if (splitChanged || routesChanged || presetChanged || boxChanged) {
            EngineHub.appLog("настройки маршрутизации изменены — переподключение")
            reconnectService()
        }
    }

    private fun reconnectService() {
        val intent = Intent(context, WisteriaVpnService::class.java)
            .setAction(WisteriaVpnService.ACTION_RECONNECT)
        // A live session means the service is already in the foreground, so
        // a plain startService is enough here; the FGS path is only a
        // fallback for vendor ROMs that refuse a background startService.
        try {
            context.startService(intent)
        } catch (t: Throwable) {
            try {
                context.startForegroundService(intent)
            } catch (t2: Throwable) {
                EngineHub.appLog("переподключение: ${t2.message ?: t.message}")
            }
        }
    }

    /** QS-tile fallback: connect the active profile, consent dialog if needed. */
    fun connectActive() {
        val id = store.activeId()
        if (id.isEmpty()) {
            EngineHub.appLog("тайл: нет активного профиля — выберите его в приложении")
            return
        }
        connect(id)
    }

    /** Deep link (wisteria://…, vless://… opened from a browser/manifest):
     *  same import path as the Links tab. */
    fun importDeepLink(uri: String): JSONObject {
        val blank = JSONObject().put("added", JSONArray()).put("errors", JSONArray())
        if (uri.isBlank()) return blank
        val res = addLinks(uri)
        val added = res.optJSONArray("added")?.length() ?: 0
        EngineHub.appLog(if (added > 0) "deep link: профиль добавлен" else "deep link: ссылка не распознана")
        return res
    }

    /** Launcher shortcut «Connect/Disconnect»: mirrors the QS tile. */
    fun toggleFromShortcut() {
        if (running()) {
            disconnect()
        } else {
            connectActive()
        }
    }

    /** Launcher shortcut for a specific profile. */
    fun connectFromShortcut(id: String) {
        if (store.profile(id) == null) {
            EngineHub.appLog("ярлык: профиль больше не существует")
            return
        }
        connect(id)
    }

    // ---- app self-update: APK from the public GitHub release ----

    /** Numeric per-segment comparison of dotted versions ("1.1.16"). */
    private fun cmpVersions(a: String, b: String): Int {
        val pa = a.trim().removePrefix("v").split(".")
        val pb = b.trim().removePrefix("v").split(".")
        for (i in 0 until maxOf(pa.size, pb.size)) {
            val x = pa.getOrNull(i)?.toIntOrNull() ?: 0
            val y = pb.getOrNull(i)?.toIntOrNull() ?: 0
            if (x != y) return x.compareTo(y)
        }
        return 0
    }

    private fun latestAppRelease(): JSONObject =
        JSONObject(httpGet("https://api.github.com/repos/qqkane/wisteria-app/releases/latest", 1 shl 20, 15000))

    private fun checkAppUpdate(): JSONObject {
        val rel = latestAppRelease()
        val latest = rel.optString("tag_name", "").removePrefix("v")
        // The release page is the only update channel now; the APK asset is
        // optional here (a tag published before the Android job finished
        // still has a usable release page).
        val htmlUrl = rel.optString("html_url", "").ifEmpty {
            if (latest.isNotEmpty()) "$RELEASES_URL/tag/v$latest" else "$RELEASES_URL/latest"
        }
        return JSONObject()
            .put("current", VERSION)
            .put("latest", latest)
            .put("has_update", cmpVersions(VERSION, latest) < 0)
            .put("url", htmlUrl)
    }

    /** App updates are handed to the browser: the release page is opened
     *  in the system browser, which downloads and installs the APK through
     *  its own trust path. The in-app downloader (unknown-sources grant,
     *  FileProvider, 90 MB into cacheDir) is gone. */
    private fun openReleasePage(target: String) {
        val clean = target.trim()
        val url = when {
            clean.startsWith("https://") || clean.startsWith("http://") -> clean
            clean.isNotEmpty() -> "$RELEASES_URL/tag/v${clean.removePrefix("v")}"
            else -> "$RELEASES_URL/latest"
        }
        val intent = Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        try {
            // context is the application context, which always works with
            // NEW_TASK even after the activity is gone.
            context.startActivity(intent)
        } catch (_: Throwable) {
            liveActivity()?.startActivity(intent)
        }
    }

    /** Status + headers + body of one GET. The header map is what makes
     *  Subscription-Userinfo / Profile-Title usable at all. */
    internal class HttpResponse(val status: Int, val headers: Map<String, String>, val body: String)

    private fun httpGet(url: String, maxBytes: Int, timeoutMs: Int): String =
        httpGetWithHeaders(url, maxBytes, timeoutMs).body

    internal fun httpGetWithHeaders(url: String, maxBytes: Int, timeoutMs: Int): HttpResponse {
        val conn = URL(url).openConnection() as HttpURLConnection
        conn.connectTimeout = timeoutMs
        conn.readTimeout = timeoutMs
        conn.instanceFollowRedirects = true
        conn.setRequestProperty("User-Agent", "WisteriaAndroid/$VERSION")
        // v2rayN/SFA servers only emit Subscription-Userinfo when the client
        // identifies as one of them; the plain UA still works everywhere.
        conn.setRequestProperty("Accept", "*/*")
        conn.setRequestProperty("X-Device-ID", store.deviceId())
        conn.setRequestProperty("X-Device-Name", store.deviceName())
        conn.setRequestProperty("X-Device-OS", store.deviceOs())
        try {
            if (conn.responseCode == 403) {
                throw err("доступ запрещён (403) — возможно, превышен лимит устройств для этой ссылки")
            }
            if (conn.responseCode !in 200..299) throw err("HTTP ${conn.responseCode}")
            val headers = HashMap<String, String>()
            for ((k, v) in conn.headerFields) {
                if (k == null || v == null || v.isEmpty()) continue
                // Multi-valued headers arrive as lists; the subscription
                // metadata fields are single-valued, so first entry is right.
                headers[k] = v[0]
            }
            val input = conn.inputStream
            val out = ByteArrayOutputStream()
            val buf = ByteArray(8192)
            var n: Int
            while (input.read(buf).also { n = it } > 0) {
                out.write(buf, 0, n)
                if (out.size() > maxBytes) {
                    val limit = if (maxBytes >= 1 shl 20) "${maxBytes shr 20} МБ" else "${maxBytes / 1024} КБ"
                    throw err("ответ больше $limit")
                }
            }
            return HttpResponse(conn.responseCode, headers, out.toString("UTF-8"))
        } finally {
            conn.disconnect()
        }
    }

    /** Subscription bodies are either a plain link list or a base64 blob
     *  (the v2rayN convention); decode only when no scheme is present.
     *  Decoding goes through SingboxLinks.b64decodeBytes(): java.util.Base64
     *  is API 26+ and minSdk is 24, so on Android 7.0/7.1 every base64
     *  subscription died with NoClassDefFoundError. */
    internal fun decodeSubBody(body: String): String {
        val trimmed = body.trim()
        if (trimmed.contains("://")) return trimmed
        val compact = trimmed.replace(Regex("\\s"), "")
        return try {
            String(SingboxLinks.b64decodeBytes(compact), Charsets.UTF_8)
        } catch (_: Exception) {
            throw err("подписка: не список ссылок и не base64")
        }
    }
}
