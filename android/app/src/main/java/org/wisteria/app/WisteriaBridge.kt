package org.wisteria.app

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.net.VpnService
import android.os.Build
import android.provider.Settings
import android.webkit.JavascriptInterface
import androidx.core.content.FileProvider
import org.json.JSONArray
import org.json.JSONObject
import org.wisteria.mobile.Mobile
import java.io.ByteArrayOutputStream
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest
import java.util.Base64

/**
 * The desktop App API re-implemented over the mobile engine: one
 * @JavascriptInterface entry (Call) dispatching JSON in/out. Return
 * shapes mirror the Go structs the shared frontend was written against
 * (core.Profile, core.Subscription, core.Status, core.LogLine,
 * core.WisteriaDiag). Thrown exceptions become {"__error": msg} and
 * surface as frontend toasts, like Wails rejections.
 */
class WisteriaBridge(private val activity: MainActivity) {

    private val context: Context get() = activity.applicationContext
    private val store by lazy { AndroidStore(context) }

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
        private const val MAX_SUB_BYTES = 4 shl 20

        /** Set when the unknown-sources settings page was opened mid-update;
         *  MainActivity.onResume continues the install once the grant lands. */
        @Volatile
        var pendingApkInstall = false

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
        "GetActiveID" -> store.activeId()
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
            activity.applyThemeFromSettings()
            maybeReconnect(old, next)
            null
        }
        "ListApps" -> listApps()
        "ListRuApps" -> listRuApps()
        "GetAutostart" -> store.autostart()
        "SetAutostart" -> { store.setAutostart(args.getBoolean(0)); null }

        // ---- desktop-only, honest stubs ----
        "IsElevated" -> false
        "DetectCores" -> JSONObject()
            .put("wisteria_found", false).put("wisteria_path", "")
            .put("singbox_found", false).put("singbox_path", "")
            .put("xray_found", false).put("xray_path", "")
        "CheckAppUpdate" -> checkAppUpdate()
        "ApplyAppUpdate" -> { startApkUpdate(); null }
        "RestartApp" -> { activity.restartApp(); null }
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
        val p = store.profile(id) ?: throw err("профиль не найден")
        store.setActive(id)
        val consent = VpnService.prepare(activity)
        if (consent != null) {
            VpnConsentHolder.pendingId = id
            activity.requestVpnConsent(id)
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
            EngineHub.pushEvent("state", WisteriaBridge.staticStatusJson("disconnected", "откройте приложение и повторите подключение"))
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
        if (id == EngineHub.profileId && isRunning(JSONObject(Mobile.status()))) {
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
        if (running.isNotEmpty() && isRunning(JSONObject(Mobile.status()))) {
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
        Thread {
            val lat = try {
                if (p.optString("type") == "wisteria") {
                    Mobile.ping(endpointOf(p))
                } else {
                    tcpPing(p.optString("host"), p.optInt("port", 443))
                }
            } catch (e: Exception) {
                -1L
            }
            p.put("latency_ms", lat)
            store.updateProfile(p)
            pushProfiles()
        }.start()
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
        Thread {
            try {
                fetchIntoSub(sub)
            } catch (e: Exception) {
                sub.put("error", e.message ?: "ошибка сети")
                store.updateSub(sub)
                EngineHub.appLog("подписка «${sub.optString("name")}»: ${e.message}")
            }
            pushProfiles()
        }.start()
    }

    /** Fetches the subscription body, parses every link (wisteria and
     *  sing-box protocols) and replaces exactly this subscription's
     *  profiles, keeping the client keys of endpoints seen before (the
     *  desktop merge rule). */
    private fun fetchIntoSub(sub: JSONObject): JSONObject {
        val body = httpGet(sub.optString("url"), MAX_SUB_BYTES, 15_000)
        val text = decodeSubBody(body)

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
        var keptActiveEndpoint: String? = null
        for (i in 0 until old.length()) {
            val p = old.getJSONObject(i)
            if (p.optString("id") == store.activeId()) {
                keptActiveEndpoint = p.optString("host") + ":" + p.optInt("port")
                break
            }
        }

        for (i in 0 until old.length()) {
            val p = old.getJSONObject(i)
            if (p.optString("id") != store.activeId()) store.deleteProfile(p.optString("id"))
        }

        var count = 0
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
        activity.runOnUiThread { cm.setPrimaryClip(clip) }
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
        Thread {
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
        }.start()
    }

    private fun externalIP(): String {
        if (!isRunning(JSONObject(Mobile.status()))) throw err("нет активного подключения")
        requestExtIP()
        return "" // the result arrives via the "extip" event
    }

    fun onConnectedCheckIP() {
        if (!store.settings().optBoolean("check_ext_ip", true)) return
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
     *  per configured endpoint, with the stored ping and reachability known
     *  only for the primary (the one host:port the engine dials). */
    private fun diagServersJson(st: JSONObject): JSONArray {
        if (st.optString("state") == "disconnected") return JSONArray()
        val prof = store.profile(EngineHub.profileId) ?: return JSONArray()
        val connected = st.optString("state") == "connected"
        val primary = endpointOf(prof)
        val latency = prof.optLong("latency_ms", 0)
        val endpoints = LinkedHashSet<String>()
        prof.optJSONArray("servers")?.let { arr ->
            for (i in 0 until arr.length()) {
                val ep = arr.getJSONObject(i).optString("endpoint")
                if (ep.isNotEmpty()) endpoints.add(ep)
            }
        }
        endpoints.add(primary) // sing-box profiles (no servers array) land here
        val out = JSONArray()
        for (ep in endpoints) {
            out.put(
                JSONObject()
                    .put("endpoint", ep)
                    .put("rtt_ms", if (ep == primary && latency > 0) latency else 0)
                    .put("reachable", connected && ep == primary)
                    .put("failures", if (ep == primary) EngineHub.retryEvents else 0)
            )
        }
        return out
    }

    // ---- helpers ----

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
     *  (MAIN/LAUNCHER) makes them visible under Android 11+ package rules. */
    private fun listApps(): JSONArray {
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
        if (splitChanged || routesChanged || presetChanged) {
            EngineHub.appLog("настройки маршрутизации изменены — переподключение")
            reconnectService()
        }
    }

    private fun reconnectService() {
        try {
            context.startForegroundService(
                Intent(context, WisteriaVpnService::class.java)
                    .setAction(WisteriaVpnService.ACTION_RECONNECT)
            )
        } catch (t: Throwable) {
            EngineHub.appLog("переподключение: ${t.message}")
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

    private fun apkAsset(rel: JSONObject): JSONObject {
        val assets = rel.optJSONArray("assets") ?: JSONArray()
        for (i in 0 until assets.length()) {
            val a = assets.getJSONObject(i)
            if (a.optString("name") == "Wisteria-android.apk") return a
        }
        throw err("в релизе нет Wisteria-android.apk")
    }

    private fun checkAppUpdate(): JSONObject {
        val rel = latestAppRelease()
        val latest = rel.optString("tag_name", "").removePrefix("v")
        val asset = apkAsset(rel)
        return JSONObject()
            .put("current", VERSION)
            .put("latest", latest)
            .put("has_update", cmpVersions(VERSION, latest) < 0)
            .put("asset_size", asset.optLong("size"))
    }

    /** The JS bridge call is synchronous from the WebView's point of view,
     *  so the download runs on its own thread and reports through apkupdate
     *  events; Call itself returns as soon as the thread is started. */
    private fun startApkUpdate() {
        Thread {
            try {
                val rel = latestAppRelease()
                val latest = rel.optString("tag_name", "").removePrefix("v")
                if (cmpVersions(VERSION, latest) >= 0) throw err("установлена актуальная версия")
                val asset = apkAsset(rel)
                if (Build.VERSION.SDK_INT >= 26 && !context.packageManager.canRequestPackageInstalls()) {
                    // Ask FIRST. The old flow downloaded 90+ MB and only then
                    // bounced the user to the settings page; the grant led to
                    // a dead end ("press again" = a full silent re-download).
                    // Now onResume continues the update automatically.
                    pendingApkInstall = true
                    context.startActivity(
                        Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:" + context.packageName))
                            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                    )
                    EngineHub.pushEvent("apkupdate", JSONObject().put("stage", "permission"))
                    return@Thread
                }
                downloadAndInstall(asset, latest)
            } catch (e: Throwable) {
                EngineHub.pushEvent("apkupdate", JSONObject().put("stage", "error").put("message", e.message ?: e.toString()))
            }
        }.start()
    }

    /** MainActivity.onResume calls this once the unknown-sources grant lands. */
    fun resumeApkUpdate() {
        Thread {
            try {
                val rel = latestAppRelease()
                val latest = rel.optString("tag_name", "").removePrefix("v")
                if (cmpVersions(VERSION, latest) >= 0) throw err("установлена актуальная версия")
                downloadAndInstall(apkAsset(rel), latest)
            } catch (e: Throwable) {
                EngineHub.pushEvent("apkupdate", JSONObject().put("stage", "error").put("message", e.message ?: e.toString()))
            }
        }.start()
    }

    private fun downloadAndInstall(asset: JSONObject, latest: String) {
        val url = asset.optString("browser_download_url")
        if (url.isEmpty()) throw err("у ассета нет ссылки на скачивание")
        val dir = File(context.cacheDir, "update")
        dir.mkdirs()
        val apk = File(dir, "Wisteria-android.apk")
        val digest = asset.optString("digest")
        // A previous attempt may have left a verified APK behind (permission
        // granted only after the download): reuse it instead of pulling the
        // whole bundle again.
        val cached = apk.exists() && digest.startsWith("sha256:") &&
            sha256Hex(apk).equals(digest.removePrefix("sha256:"), ignoreCase = true)
        if (!cached) {
            EngineHub.pushEvent(
                "apkupdate", JSONObject().put("stage", "downloading").put("latest", latest).put("pct", 0)
            )
            downloadToFile(url, apk, asset.optLong("size"))
            // GitHub publishes a sha256 digest per asset; a mismatch means a
            // corrupted or tampered download — never install it.
            if (digest.startsWith("sha256:")) {
                val got = sha256Hex(apk)
                if (!got.equals(digest.removePrefix("sha256:"), ignoreCase = true)) {
                    apk.delete()
                    throw err("контрольная сумма APK не совпала — файл удалён")
                }
            }
        }
        installApk(apk)
        EngineHub.pushEvent("apkupdate", JSONObject().put("stage", "install"))
    }

    private fun downloadToFile(url: String, dst: File, expected: Long) {
        val conn = URL(url).openConnection() as HttpURLConnection
        conn.connectTimeout = 15000
        conn.readTimeout = 60000
        conn.instanceFollowRedirects = true
        conn.setRequestProperty("User-Agent", "WisteriaAndroid/$VERSION")
        try {
            if (conn.responseCode !in 200..299) throw err("HTTP ${conn.responseCode} при скачивании APK")
            val total = if (expected > 0) expected else conn.contentLengthLong
            var done = 0L
            var lastPct = -1
            conn.inputStream.use { input ->
                dst.outputStream().use { out ->
                    val buf = ByteArray(64 * 1024)
                    while (true) {
                        val n = input.read(buf)
                        if (n <= 0) break
                        out.write(buf, 0, n)
                        done += n
                        if (total > 0) {
                            val pct = (done * 100 / total).toInt().coerceAtMost(100)
                            if (pct != lastPct) {
                                lastPct = pct
                                EngineHub.pushEvent(
                                    "apkupdate", JSONObject().put("stage", "downloading").put("pct", pct)
                                )
                            }
                        }
                    }
                }
            }
        } finally {
            conn.disconnect()
        }
    }

    private fun sha256Hex(f: File): String {
        val md = MessageDigest.getInstance("SHA-256")
        f.inputStream().use { input ->
            val buf = ByteArray(64 * 1024)
            while (true) {
                val n = input.read(buf)
                if (n <= 0) break
                md.update(buf, 0, n)
            }
        }
        return md.digest().joinToString("") { "%02x".format(it) }
    }

    private fun installApk(apk: File) {
        if (Build.VERSION.SDK_INT >= 26 && !context.packageManager.canRequestPackageInstalls()) {
            pendingApkInstall = true
            context.startActivity(
                Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:" + context.packageName))
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            )
            throw err("разрешите установку из неизвестных источников — обновление продолжится автоматически")
        }
        val uri = FileProvider.getUriForFile(context, context.packageName + ".fileprovider", apk)
        context.startActivity(
            Intent(Intent.ACTION_VIEW)
                .setDataAndType(uri, "application/vnd.android.package-archive")
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
        )
    }

    private fun httpGet(url: String, maxBytes: Int, timeoutMs: Int): String {
        val conn = URL(url).openConnection() as HttpURLConnection
        conn.connectTimeout = timeoutMs
        conn.readTimeout = timeoutMs
        conn.instanceFollowRedirects = true
        conn.setRequestProperty("User-Agent", "WisteriaAndroid/$VERSION")
        try {
            if (conn.responseCode !in 200..299) throw err("HTTP ${conn.responseCode}")
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
            return out.toString("UTF-8")
        } finally {
            conn.disconnect()
        }
    }

    /** Subscription bodies are either a plain link list or a base64 blob
     *  (the v2rayN convention); decode only when no scheme is present. */
    private fun decodeSubBody(body: String): String {
        val trimmed = body.trim()
        if (trimmed.contains("://")) return trimmed
        val compact = trimmed.replace(Regex("\\s"), "")
        return try {
            val padded = compact.padEnd((compact.length + 3) / 4 * 4, '=')
            String(Base64.getDecoder().decode(padded), Charsets.UTF_8)
        } catch (_: Exception) {
            try {
                String(Base64.getUrlDecoder().decode(compact), Charsets.UTF_8)
            } catch (_: Exception) {
                throw err("подписка: не список ссылок и не base64")
            }
        }
    }
}
