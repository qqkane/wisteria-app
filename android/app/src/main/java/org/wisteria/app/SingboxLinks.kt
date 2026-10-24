package org.wisteria.app

import android.net.Uri
import org.json.JSONArray
import org.json.JSONObject

/**
 * Share-link support for the sing-box protocols (vless, vmess, trojan,
 * shadowsocks, hysteria2, tuic): parses the URI formats v2rayN / SFA /
 * Hiddify produce into profile records, and turns a profile back into a
 * sing-box config (tun inbound + the parsed outbound). The wisteria
 * protocol keeps its own engine and parser.
 */
object SingboxLinks {

    val SCHEMES = setOf("vless", "vmess", "trojan", "ss", "hysteria2", "hy2", "tuic")

    fun handles(link: String): Boolean {
        val scheme = link.substringBefore("://", "").lowercase()
        return scheme in SCHEMES
    }

    /** Profile record for AndroidStore: name, type, host, port, raw. */
    fun parseProfile(link: String): JSONObject? {
        return try {
            when (link.substringBefore("://").lowercase()) {
                "vmess" -> parseVmess(link)
                "ss" -> parseSs(link)
                else -> parseUriStyle(link)
            }
        } catch (_: Exception) {
            null
        }
    }

    // ---- uri-style: vless / trojan / hysteria2 / tuic -------------------

    private fun parseUriStyle(link: String): JSONObject? {
        val uri = Uri.parse(link)
        val scheme = uri.scheme?.lowercase() ?: return null
        if (scheme !in SCHEMES) return null
        val host = uri.host ?: return null
        val port = if (uri.port in 1..65535) uri.port else 443
        val name = uri.fragment?.takeIf { it.isNotBlank() } ?: "$host:$port"
        return JSONObject()
            .put("name", name)
            .put("type", if (scheme == "hy2") "hysteria2" else scheme)
            .put("host", host)
            .put("port", port)
            .put("raw", link)
    }

    // ---- vmess: vmess://base64(v2rayN JSON) ------------------------------

    private fun parseVmess(link: String): JSONObject? {
        val payload = link.removePrefix("vmess://").substringBefore("#")
        val j = JSONObject(b64decode(payload))
        val host = j.optString("add")
        if (host.isEmpty()) return null
        val port = j.optString("port").toIntOrNull() ?: 443
        val name = j.optString("ps").ifEmpty { "$host:$port" }
        return JSONObject()
            .put("name", name)
            .put("type", "vmess")
            .put("host", host)
            .put("port", port)
            .put("raw", link)
    }

    // ---- shadowsocks: SIP002 and the legacy all-base64 form -------------

    private fun parseSs(link: String): JSONObject? {
        val uri = Uri.parse(link)
        val name = uri.fragment?.takeIf { it.isNotBlank() }
        var body = (uri.schemeSpecificPart ?: return null).substringBefore("#")
        val plugin = uri.getQueryParameter("plugin").orEmpty()

        val host: String
        val port: Int
        val method: String
        val password: String
        if (body.contains("@")) {
            val userinfo = body.substringBeforeLast("@")
            val hostPart = body.substringAfterLast("@")
            val decoded = runCatching { b64decode(userinfo) }.getOrNull()
            val cred = decoded ?: java.net.URLDecoder.decode(userinfo, "UTF-8")
            method = cred.substringBefore(":")
            password = cred.substringAfter(":", missingDelimiterValue = "")
            val (h, p) = splitHostPort(hostPart)
            host = h
            port = p
        } else {
            body = body.substringBefore("?")
            val decoded = b64decode(body)
            val cred = decoded.substringBeforeLast("@")
            val hostPart = decoded.substringAfterLast("@")
            method = cred.substringBefore(":")
            password = cred.substringAfter(":", missingDelimiterValue = "")
            val (h, p) = splitHostPort(hostPart)
            host = h
            port = p
        }
        if (host.isEmpty() || method.isEmpty() || password.isEmpty()) return null

        val cleanName = name ?: "$host:$port"
        return JSONObject()
            .put("name", cleanName)
            .put("type", "ss")
            .put("host", host)
            .put("port", port)
            .put("raw", link)
            .put("_plugin", plugin)
    }

    /** "host:port" / "[v6]:port" → (host, port); port defaults to 443. */
    private fun splitHostPort(part0: String): Pair<String, Int> {
        val part = part0.substringBefore("/").substringBefore("?")
        if (part.startsWith("[")) {
            val host = part.substringAfter("[").substringBefore("]")
            val port = part.substringAfterLast(":", "").toIntOrNull() ?: 443
            return host to port
        }
        val idx = part.lastIndexOf(':')
        if (idx <= 0) return part to 443
        val port = part.substring(idx + 1).toIntOrNull() ?: return part to 443
        return part.substring(0, idx) to port
    }

    // ---- base64 (portable decoder: android.util.Base64 is JVM-untestable,
    //      java.util.Base64 is API 26+; minSdk is 24) -----------------------

    private const val ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
    private val INV = IntArray(128) { -1 }.also { inv ->
        for (i in ALPHABET.indices) inv[ALPHABET[i].code] = i
        inv['-'.code] = ALPHABET.indexOf('+')
        inv['_'.code] = ALPHABET.indexOf('/')
    }

    fun b64decode(s0: String): String {
        var s = s0.trim().replace("\n", "").replace("\r", "")
        while (s.length % 4 != 0) s += "="
        val out = java.io.ByteArrayOutputStream()
        var buffer = 0
        var bits = 0
        for (c in s) {
            if (c == '=') break
            if (c.code >= 128) throw IllegalArgumentException("bad base64")
            val v = INV[c.code]
            if (v < 0) throw IllegalArgumentException("bad base64 char $c")
            buffer = (buffer shl 6) or v
            bits += 6
            if (bits >= 8) {
                bits -= 8
                out.write((buffer shr bits) and 0xFF)
            }
        }
        // The payload is raw bytes, not chars: vmess "ps" names carry UTF-8.
        return String(out.toByteArray(), Charsets.UTF_8)
    }

    // ---- config builder ---------------------------------------------------

    /** Private ranges for the bypass-LAN rule (applied before the proxy). */
    private val PRIVATE_CIDRS = listOf(
        "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8",
        "169.254.0.0/16", "100.64.0.0/10", "224.0.0.0/4",
        "::1/128", "fe80::/10", "fc00::/7",
    )

    /** Full sing-box config for the profile's raw link: tun inbound, the
     *  parsed outbound, DNS and route. The optional settings carry the
     *  routing tab (bypass_lan, route_direct, route_block, remote_dns,
     *  log_level) — the same keys the desktop saves. */
    fun buildConfig(raw: String, name: String, settings: JSONObject? = null): String {
        val s = settings ?: JSONObject()
        val outbound = buildOutbound(raw, name)

        val blockDomains = JSONArray()
        val blockIps = JSONArray()
        val directDomains = JSONArray()
        val directIps = JSONArray()
        for ((value, isIp) in parseRouteList(s.optString("route_block"))) {
            (if (isIp) blockIps else blockDomains).put(value)
        }
        for ((value, isIp) in parseRouteList(s.optString("route_direct"))) {
            (if (isIp) directIps else directDomains).put(value)
        }
        val hasBlock = blockDomains.length() > 0 || blockIps.length() > 0

        val rules = JSONArray()
            .put(JSONObject().put("action", "sniff"))
            .put(JSONObject().put("protocol", "dns").put("action", "hijack-dns"))
        if (hasBlock) {
            rules.put(JSONObject().put("outbound", "block").apply {
                if (blockDomains.length() > 0) put("domain_suffix", blockDomains)
                if (blockIps.length() > 0) put("ip_cidr", blockIps)
            })
        }
        if (directDomains.length() > 0 || directIps.length() > 0) {
            rules.put(JSONObject().put("outbound", "direct").apply {
                if (directDomains.length() > 0) put("domain_suffix", directDomains)
                if (directIps.length() > 0) put("ip_cidr", directIps)
            })
        }
        if (s.optBoolean("bypass_lan", false)) {
            rules.put(
                JSONObject()
                    .put("ip_cidr", JSONArray().apply { PRIVATE_CIDRS.forEach { put(it) } })
                    .put("outbound", "direct"),
            )
        }

        val outbounds = JSONArray()
            .put(outbound.put("domain_resolver", "dns-remote"))
            .put(JSONObject().put("type", "direct").put("tag", "direct"))
        if (hasBlock) {
            outbounds.put(JSONObject().put("type", "block").put("tag", "block"))
        }

        val cfg = JSONObject()
            .put(
                "log",
                JSONObject().put("level", s.optString("log_level").ifEmpty { "info" }).put("timestamp", true),
            )
            .put(
                "dns",
                JSONObject()
                    .put("servers", JSONArray().put(parseDnsServer(s.optString("remote_dns"))))
                    .put("final", "dns-remote")
                    .put("strategy", "prefer_ipv4")
                    .put("independent_cache", true),
            )
            .put(
                "inbounds",
                JSONArray().put(
                    JSONObject()
                        .put("type", "tun")
                        .put("tag", "tun-in")
                        .put("mtu", 1400)
                        .put("address", JSONArray().put("172.19.0.1/30").put("fdfe:dcba:9876::1/126"))
                        .put("auto_route", true)
                        .put("strict_route", false)
                        .put("stack", "mixed"),
                ),
            )
            .put("outbounds", outbounds)
            .put(
                "route",
                JSONObject()
                    .put("auto_detect_interface", true)
                    .put("rules", rules)
                    .put("final", "proxy"),
            )
        return cfg.toString()
    }

    /** "tcp://1.1.1.1" / "udp://8.8.8.8" / "tls://…" / bare address → the
     *  sing-box dns-server entry (udp 8.8.8.8 when unset). */
    private fun parseDnsServer(raw0: String): JSONObject {
        var raw = raw0.trim()
        if (raw.isEmpty()) raw = "udp://8.8.8.8"
        val type = when {
            raw.startsWith("tcp://") -> "tcp"
            raw.startsWith("tls://") -> "tls"
            raw.startsWith("https://") -> "https"
            else -> "udp"
        }
        val server = raw.substringAfter("://").substringBefore('/').trim().ifEmpty { "8.8.8.8" }
        return JSONObject().put("type", type).put("tag", "dns-remote").put("server", server)
    }

    /** Route lists: one entry per line (or comma/semicolon), # and //
     *  comments stripped. Returns value + whether it's an IP/CIDR. */
    private fun parseRouteList(text: String): List<Pair<String, Boolean>> {
        val out = mutableListOf<Pair<String, Boolean>>()
        val ipLike = Regex("^[0-9a-fA-F:.]+(/\\d{1,3})?$")
        for (rawLine in text.split("\n", ",", ";")) {
            var line = rawLine.substringBefore("#").substringBefore("//").trim()
            if (line.isEmpty()) continue
            out.add(line.lowercase() to ipLike.matches(line))
        }
        return out
    }

    private fun buildOutbound(raw: String, name: String): JSONObject {
        return when (raw.substringBefore("://").lowercase()) {
            "vmess" -> vmessOutbound(raw, name)
            "ss" -> ssOutbound(raw, name)
            "vless" -> vlessOutbound(raw, name)
            "trojan" -> trojanOutbound(raw, name)
            "hysteria2", "hy2" -> hy2Outbound(raw, name)
            "tuic" -> tuicOutbound(raw, name)
            else -> throw IllegalArgumentException("неизвестный протокол")
        }
    }

    private fun base(outboundType: String, uri: Uri, fallbackPort: Int): JSONObject {
        val host = uri.host ?: throw IllegalArgumentException("нет адреса сервера")
        val port = if (uri.port in 1..65535) uri.port else fallbackPort
        return JSONObject()
            .put("type", outboundType)
            .put("tag", "proxy")
            .put("server", host)
            .put("server_port", port)
    }

    private fun tlsBlock(uri: Uri, defaultName: String?, forced: Boolean): JSONObject? {
        val security = uri.getQueryParameter("security")?.lowercase()
        val sni = uri.getQueryParameter("sni") ?: defaultName
        val insecure = (uri.getQueryParameter("insecure") ?: uri.getQueryParameter("allowInsecure")
            ?: uri.getQueryParameter("allow_insecure")) in setOf("1", "true")
        val reality = security == "reality"
        if (!forced && security != "tls" && !reality) return null
        val tls = JSONObject().put("enabled", true)
        if (!sni.isNullOrBlank()) tls.put("server_name", sni)
        if (insecure) tls.put("insecure", true)
        val alpn = uri.getQueryParameter("alpn")
        if (!alpn.isNullOrBlank()) {
            tls.put("alpn", JSONArray().apply { alpn.split(",").forEach { if (it.isNotBlank()) put(it.trim()) } })
        }
        val fp = uri.getQueryParameter("fp")
        if (!fp.isNullOrBlank()) {
            tls.put("utls", JSONObject().put("enabled", true).put("fingerprint", fp))
        }
        if (reality) {
            tls.put(
                "reality",
                JSONObject()
                    .put("enabled", true)
                    .put("public_key", uri.getQueryParameter("pbk").orEmpty())
                    .put("short_id", uri.getQueryParameter("sid").orEmpty()),
            )
        }
        return tls
    }

    private fun transportBlock(uri: Uri): JSONObject? {
        val net = uri.getQueryParameter("type")?.lowercase() ?: return null
        val path = uri.getQueryParameter("path")
        val host = uri.getQueryParameter("host")
        return when (net) {
            "ws" -> {
                val t = JSONObject().put("type", "ws")
                if (!path.isNullOrBlank()) t.put("path", path)
                if (!host.isNullOrBlank()) {
                    t.put("headers", JSONObject().put("Host", host))
                }
                t
            }
            "http", "h2" -> {
                val t = JSONObject().put("type", "http")
                if (!path.isNullOrBlank()) t.put("path", path)
                if (!host.isNullOrBlank()) {
                    t.put("headers", JSONObject().put("Host", host))
                }
                t
            }
            "httpupgrade" -> {
                val t = JSONObject().put("type", "httpupgrade")
                if (!path.isNullOrBlank()) t.put("path", path)
                if (!host.isNullOrBlank()) {
                    t.put("headers", JSONObject().put("Host", host))
                }
                t
            }
            "grpc" -> {
                val t = JSONObject().put("type", "grpc")
                val sn = uri.getQueryParameter("serviceName")
                if (!sn.isNullOrBlank()) t.put("service_name", sn)
                t
            }
            else -> null // tcp / kcp / quic raw
        }
    }

    private fun vlessOutbound(raw: String, name: String): JSONObject {
        val uri = Uri.parse(raw)
        val o = base("vless", uri, 443)
            .put("uuid", uri.userInfo ?: throw IllegalArgumentException("vless: нет uuid"))
            .put("packet_encoding", "xudp")
        uri.getQueryParameter("flow")?.takeIf { it.isNotBlank() }?.let { o.put("flow", it) }
        tlsBlock(uri, uri.host, false)?.let { o.put("tls", it) }
        transportBlock(uri)?.let { o.put("transport", it) }
        return o
    }

    private fun trojanOutbound(raw: String, name: String): JSONObject {
        val uri = Uri.parse(raw)
        val o = base("trojan", uri, 443)
            .put("password", java.net.URLDecoder.decode(uri.userInfo ?: "", "UTF-8"))
        tlsBlock(uri, uri.host, true)?.let { o.put("tls", it) }
        transportBlock(uri)?.let { o.put("transport", it) }
        return o
    }

    private fun hy2Outbound(raw: String, name: String): JSONObject {
        val uri = Uri.parse(raw)
        val o = base("hysteria2", uri, 443)
            .put("password", java.net.URLDecoder.decode(uri.userInfo ?: "", "UTF-8"))
        uri.getQueryParameter("obfs")?.takeIf { it.isNotBlank() }?.let {
            o.put(
                "obfs",
                JSONObject().put("type", it).put("password", uri.getQueryParameter("obfs-password").orEmpty()),
            )
        }
        val tls = tlsBlock(uri, uri.host, true) ?: JSONObject().put("enabled", true)
        if (!tls.has("alpn")) tls.put("alpn", JSONArray().put("h3"))
        o.put("tls", tls)
        return o
    }

    private fun tuicOutbound(raw: String, name: String): JSONObject {
        val uri = Uri.parse(raw)
        val userinfo = uri.userInfo ?: throw IllegalArgumentException("tuic: нет учётных данных")
        val o = base("tuic", uri, 443)
            .put("uuid", java.net.URLDecoder.decode(userinfo.substringBefore(":"), "UTF-8"))
        userinfo.substringAfter(":", missingDelimiterValue = "").takeIf { it.isNotEmpty() }?.let {
            o.put("password", java.net.URLDecoder.decode(it, "UTF-8"))
        }
        uri.getQueryParameter("congestion_control")?.takeIf { it.isNotBlank() }?.let { o.put("congestion_control", it) }
        uri.getQueryParameter("udp_relay_mode")?.takeIf { it.isNotBlank() }?.let { o.put("udp_relay_mode", it) }
        val tls = tlsBlock(uri, uri.host, true) ?: JSONObject().put("enabled", true)
        if (!tls.has("alpn")) tls.put("alpn", JSONArray().put("h3"))
        o.put("tls", tls)
        return o
    }

    private fun vmessOutbound(raw: String, name: String): JSONObject {
        val j = JSONObject(b64decode(raw.removePrefix("vmess://").substringBefore("#")))
        val host = j.optString("add")
        val port = j.optString("port").toIntOrNull() ?: 443
        val o = JSONObject()
            .put("type", "vmess")
            .put("tag", "proxy")
            .put("server", host)
            .put("server_port", port)
            .put("uuid", j.optString("id"))
            .put("security", j.optString("scy").ifEmpty { "auto" })
            .put("alter_id", j.optString("aid").toIntOrNull() ?: 0)
        if (j.optString("tls") == "tls" || j.optString("tls") == "reality") {
            val tls = JSONObject().put("enabled", true)
            val sni = j.optString("sni")
            if (sni.isNotEmpty()) tls.put("server_name", sni)
            val fp = j.optString("fp")
            if (fp.isNotEmpty()) tls.put("utls", JSONObject().put("enabled", true).put("fingerprint", fp))
            val alpn = j.optString("alpn")
            if (alpn.isNotEmpty()) {
                tls.put("alpn", JSONArray().apply { alpn.split(",").forEach { if (it.isNotBlank()) put(it.trim()) } })
            }
            if (j.optString("tls") == "reality") {
                tls.put(
                    "reality",
                    JSONObject()
                        .put("enabled", true)
                        .put("public_key", j.optString("pbk"))
                        .put("short_id", j.optString("sid")),
                )
            }
            o.put("tls", tls)
        }
        val net = j.optString("net")
        val transport = when (net) {
            "ws" -> JSONObject().put("type", "ws").put("path", j.optString("path").ifEmpty { "/" }).apply {
                if (j.optString("host").isNotEmpty()) put("headers", JSONObject().put("Host", j.optString("host")))
            }
            "h2", "http" -> JSONObject().put("type", "http").put("path", j.optString("path").ifEmpty { "/" }).apply {
                if (j.optString("host").isNotEmpty()) put("headers", JSONObject().put("Host", j.optString("host")))
            }
            "httpupgrade" -> JSONObject().put("type", "httpupgrade").put("path", j.optString("path").ifEmpty { "/" })
            "grpc" -> JSONObject().put("type", "grpc").put("service_name", j.optString("path"))
            else -> null
        }
        if (transport != null) o.put("transport", transport)
        return o
    }

    private fun ssOutbound(raw: String, name: String): JSONObject {
        val uri = Uri.parse(raw)
        var body = (uri.schemeSpecificPart ?: "").substringBefore("#").substringBefore("?")
        val o = JSONObject().put("type", "shadowsocks").put("tag", "proxy")
        if (body.contains("@")) {
            val userinfo = body.substringBeforeLast("@")
            val hostPart = body.substringAfterLast("@")
            val cred = runCatching { b64decode(userinfo) }.getOrDefault(
                java.net.URLDecoder.decode(userinfo, "UTF-8"),
            )
            val (h, p) = splitHostPort(hostPart)
            o.put("method", cred.substringBefore(":"))
                .put("password", cred.substringAfter(":", missingDelimiterValue = ""))
                .put("server", h)
                .put("server_port", p)
        } else {
            val decoded = b64decode(body)
            val cred = decoded.substringBeforeLast("@")
            val hostPart = decoded.substringAfterLast("@")
            val (h, p) = splitHostPort(hostPart)
            o.put("method", cred.substringBefore(":"))
                .put("password", cred.substringAfter(":", missingDelimiterValue = ""))
                .put("server", h)
                .put("server_port", p)
        }
        uri.getQueryParameter("plugin")?.takeIf { it.isNotBlank() }?.let { plugin ->
            val parts = plugin.split(";")
            o.put("plugin", parts[0])
            if (parts.size > 1) o.put("plugin_opts", parts.drop(1).joinToString(";"))
        }
        return o
    }
}
