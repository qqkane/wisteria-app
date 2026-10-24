package org.wisteria.app

import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.webkit.JavascriptInterface
import org.json.JSONArray
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.net.HttpURLConnection
import java.net.URL
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

    companion object {
        const val VERSION = "0.2.0"
        private const val MAX_SUB_BYTES = 4 shl 20
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
        "GetWisteriaDiag" -> diagJson()
        "Version" -> VERSION
        "GetExternalIP" -> externalIP()

        // ---- profiles ----
        "ListProfiles" -> store.listProfiles()
        "GetActiveID" -> store.activeId()
        "SetActive" -> { store.setActive(args.getString(0)); null }
        "Connect" -> { connect(args.getString(0)); null }
        "Disconnect" -> { disconnect(); null }
        "DeleteProfile" -> { deleteProfile(args.getString(0)); null }
        "PingProfile" -> { pingProfile(args.getString(0)); null }
        "AddLinks" -> addLinks(args.getString(0))
        "AddManual" -> addManual(args.getJSONObject(0))
        "ImportTOML" -> throw err("Импорт TOML пока только в десктоп-версии — используйте ссылку wisteria://")

        // ---- subscriptions ----
        "ListSubscriptions" -> store.listSubs()
        "AddSubscription" -> addSubscription(args.getString(0))
        "RefreshSubscription" -> refreshSubscription(args.getString(0))
        "DeleteSubscription" -> { store.deleteSub(args.getString(0)); null }
        "ImportFromClipboard" -> importFromClipboard()

        // ---- settings ----
        "GetSettings" -> store.settings()
        "SaveSettings" -> { store.saveSettings(args.getJSONObject(0)); null }
        "GetAutostart" -> store.autostart()
        "SetAutostart" -> { store.setAutostart(args.getBoolean(0)); null }

        // ---- desktop-only, honest stubs ----
        "IsElevated" -> false
        "DetectCores" -> JSONObject()
            .put("wisteria_found", false).put("wisteria_path", "")
            .put("singbox_found", false).put("singbox_path", "")
            .put("xray_found", false).put("xray_path", "")
        "CheckAppUpdate" -> JSONObject().put("current", VERSION).put("latest", VERSION).put("has_update", false)
        "ApplyAppUpdate" -> throw err("Обновление APK ставится вручную — скачайте новый из релиза")
        "RestartApp" -> { activity.restartApp(); null }
        "CheckCoreUpdate" -> JSONObject().put("current", "-").put("latest", "-").put("has_update", false)
        "UpdateCore" -> throw err("Ядра sing-box/xray недоступны на Android")

        else -> throw err("неизвестный метод: $name")
    }

    // ---- state ----

    /** Engine state mapped onto core.Status (the desktop struct). */
    fun statusJson(): JSONObject {
        val engine = JSONObject(Mobile.status())
        val running = isRunning(engine)
        val out = JSONObject()
        out.put("state", mapState(engine.optString("state")))
        out.put("profile_id", if (running) EngineHub.profileId else "")
        out.put("profile_name", if (running) EngineHub.profileName else "")
        out.put("type", if (running) "wisteria" else "")
        val since = engine.optLong("since", 0)
        out.put("started_at", if (since > 0) AndroidStore.epochIso(since) else "")
        out.put("sys_proxy", false)
        out.put("tun", false)
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
        if (p.optString("type") != "wisteria") {
            throw err("протокол «${p.optString("type")}» на Android пока не поддерживается — только wisteria")
        }
        store.setActive(id)
        val consent = VpnService.prepare(activity)
        if (consent != null) {
            activity.requestVpnConsent(id)
            return
        }
        startConnectService(id)
    }

    private fun startConnectService(id: String) {
        val intent = Intent(context, WisteriaVpnService::class.java)
            .setAction(WisteriaVpnService.ACTION_CONNECT)
            .putExtra(WisteriaVpnService.EXTRA_PROFILE_ID, id)
        context.startForegroundService(intent)
    }

    private fun disconnect() {
        context.startService(
            Intent(context, WisteriaVpnService::class.java)
                .setAction(WisteriaVpnService.ACTION_DISCONNECT)
        )
    }

    /** Called by the activity after the VPN consent dialog resolved OK. */
    fun onVpnConsentGranted(profileId: String) {
        startConnectService(profileId)
    }

    // ---- profiles ----

    private fun deleteProfile(id: String) {
        if (id == EngineHub.profileId && isRunning(JSONObject(Mobile.status()))) {
            throw err("профиль подключён — сначала отключитесь")
        }
        if (!store.deleteProfile(id)) throw err("профиль не найден")
        EngineHub.appLog("профиль удалён")
        pushProfiles()
    }

    private fun pingProfile(id: String) {
        val p = store.profile(id) ?: throw err("профиль не найден")
        val lat = try {
            Mobile.ping(endpointOf(p))
        } catch (e: Exception) {
            -1L
        }
        p.put("latency_ms", lat)
        store.updateProfile(p)
        pushProfiles()
    }

    private fun addLinks(text: String): JSONObject {
        val added = JSONArray()
        val errors = JSONArray()
        for (rawLine in text.split("\n")) {
            val line = rawLine.trim()
            if (line.isEmpty()) continue
            if (!line.contains("://")) continue
            try {
                val entries = JSONArray(Mobile.parseLinks(line))
                for (j in 0 until entries.length()) {
                    val entry = entries.getJSONObject(j)
                    if (entry.optString("key").isEmpty()) {
                        errors.put(JSONObject().put("line", line).put("error", "приглашение без ключа: Android-версия работает по ссылкам с ключом (Ссылка клиенту в панели)"))
                        continue
                    }
                    val servers = entry.optJSONArray("servers")
                    if (servers == null || servers.length() == 0) {
                        errors.put(JSONObject().put("line", line).put("error", "в ссылке нет серверов"))
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
                    val saved = store.addProfile(p)
                    if (saved != null) added.put(saved)
                }
            } catch (e: Exception) {
                errors.put(JSONObject().put("line", line).put("error", e.message ?: "не распознано"))
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
        val saved = store.addProfile(p) ?: throw err("такой профиль уже есть")
        EngineHub.appLog("профиль «${saved.optString("name")}» добавлен вручную")
        pushProfiles()
        return saved
    }

    // ---- subscriptions ----

    private fun addSubscription(url: String): JSONObject {
        val sub = store.addSub(url.trim()) ?: throw err("такая подписка уже есть")
        EngineHub.appLog("подписка добавлена: ${sub.optString("name")}")
        return try {
            fetchIntoSub(sub)
        } catch (e: Exception) {
            sub.put("error", e.message ?: "ошибка сети")
            store.updateSub(sub)
            sub
        }
    }

    private fun refreshSubscription(id: String): JSONObject {
        val sub = store.sub(id) ?: throw err("подписка не найдена")
        return try {
            fetchIntoSub(sub)
        } catch (e: Exception) {
            sub.put("error", e.message ?: "ошибка сети")
            store.updateSub(sub)
            sub
        }
    }

    /** Fetches the subscription body, parses every wisteria link and
     *  replaces exactly this subscription's profiles, keeping the client
     *  keys of endpoints seen before (the desktop merge rule). */
    private fun fetchIntoSub(sub: JSONObject): JSONObject {
        val body = httpGet(sub.optString("url"), MAX_SUB_BYTES, 15_000)
        val text = decodeSubBody(body)
        val entries = JSONArray(Mobile.parseLinks(text))

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
                        try {
                            fetchIntoSub(sub)
                        } catch (e: Exception) {
                            sub.put("error", e.message ?: "ошибка сети")
                            store.updateSub(sub)
                            EngineHub.appLog("подписка ${sub.optString("name")}: ${e.message}")
                        }
                    }
                } catch (e: Exception) {
                    EngineHub.appLog("подписка $line: ${e.message}")
                }
            } else {
                links.append(line).append("\n")
            }
        }
        val res = if (links.isNotEmpty()) addLinks(links.toString())
        else JSONObject().put("added", JSONArray()).put("errors", JSONArray())
        return res.put("subscriptions", subCount)
    }

    // ---- external IP / diag ----

    private fun externalIP(): String {
        if (!isRunning(JSONObject(Mobile.status()))) throw err("нет активного подключения")
        val ip = httpGet("https://api.ipify.org", 1024, 10_000).trim()
        if (ip.isEmpty()) throw err("пустой ответ")
        EngineHub.pushEvent("extip", JSONObject().put("ip", ip))
        return ip
    }

    fun onConnectedCheckIP() {
        if (!store.settings().optBoolean("check_ext_ip", true)) return
        Thread {
            try {
                val ip = httpGet("https://api.ipify.org", 1024, 10_000).trim()
                if (ip.isNotEmpty()) EngineHub.pushEvent("extip", JSONObject().put("ip", ip))
            } catch (e: Exception) {
                EngineHub.pushEvent("extip", JSONObject().put("error", e.message ?: ""))
            }
        }.start()
    }

    private fun diagJson(): JSONObject {
        val st = statusJson()
        val active = st.optString("state") != "disconnected"
        val out = JSONObject()
            .put("active", active)
            .put("status", st)
            .put("servers", JSONArray())
            .put("updated", System.nanoTime())
        val events = EngineHub.diagEventsJson()
        out.put("events", events)
        var lastKind = ""
        if (events.length() > 0) lastKind = events.getJSONObject(events.length() - 1).optString("kind")
        out.put("last_kind", lastKind)
        return out
    }

    // ---- helpers ----

    private fun pushProfiles() {
        EngineHub.pushEvent("profiles", null)
    }

    fun endpointOf(p: JSONObject): String {
        val host = p.optString("host")
        val port = p.optInt("port", 0)
        return if (port > 0) "$host:$port" else host
    }

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
                if (out.size() > maxBytes) throw err("ответ больше 4 МБ")
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
