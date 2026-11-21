package org.wisteria.app

import org.json.JSONArray
import org.json.JSONObject
import java.io.ByteArrayOutputStream

/**
 * Share-link support for the sing-box protocols (vless, vmess, trojan,
 * shadowsocks, hysteria2, tuic): parses the URI formats v2rayN / SFA /
 * Hiddify produce into profile records, and turns a profile back into a
 * sing-box config (tun inbound + the parsed outbound). The wisteria
 * protocol keeps its own engine and parser.
 *
 * URI handling is done by [LinkParser] instead of android.net.Uri. Two
 * reasons: android.net.Uri is a stub on the JVM (nothing here could be unit
 * tested), and its userInfo/host accessors hide which decoding happened —
 * the hysteria2 outbound ran the already-decoded userInfo through
 * URLDecoder a second time and corrupted every password containing '+' or
 * '%'. [LinkParser] percent-decodes exactly once and never turns '+' into a
 * space (that is form encoding, not URI encoding).
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
        val uri = LinkParser.parse(link) ?: return null
        val scheme = uri.scheme
        if (scheme !in SCHEMES) return null
        val host = uri.host
        if (host.isEmpty()) return null
        val port = if (uri.port in 1..65535) uri.port else 443
        val name = uri.fragment.takeIf { it.isNotBlank() } ?: "$host:$port"
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
        val uri = LinkParser.parse(link)
        val name = uri?.fragment?.takeIf { it.isNotBlank() }
        var body = rawSsBody(link)
        val plugin = uri?.param("plugin").orEmpty()

        val host: String
        val port: Int
        val method: String
        val password: String
        if (body.contains("@")) {
            val userinfo = body.substringBeforeLast("@")
            val hostPart = body.substringAfterLast("@")
            val decoded = runCatching { b64decode(userinfo) }.getOrNull()
            val cred = decoded ?: percentDecode(userinfo)
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

    /** The ss payload (userinfo@host:port) taken from the RAW link:
     *  Uri.schemeSpecificPart is percent-decoded and keeps the leading "//"
     *  of hierarchical URIs — both corrupt base64 credentials, and on-device
     *  every SIP002 and legacy ss:// import failed with «не удалось разобрать
     *  ссылку». Kept as raw string slicing even now that LinkParser exists:
     *  a legacy blob may legally contain '/' and '?', which a URI parser
     *  would take for path/query separators. */
    private fun rawSsBody(link: String): String =
        link.substringAfter("://", "").substringBefore("#").substringBefore("?")

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

    // ---- percent decoding -------------------------------------------------

    /**
     * Decode %XX escapes once, into UTF-8. '+' is left alone: it is a
     * literal plus in a URI, and decoding it as a space was exactly the
     * corruption the hysteria2 outbound suffered from.
     */
    internal fun percentDecode(s: String): String {
        if (s.indexOf('%') < 0) return s
        val out = ByteArrayOutputStream(s.length)
        var i = 0
        while (i < s.length) {
            val c = s[i]
            if (c == '%' && i + 2 < s.length) {
                val hi = hexVal(s[i + 1])
                val lo = hexVal(s[i + 2])
                if (hi >= 0 && lo >= 0) {
                    out.write((hi shl 4) or lo)
                    i += 3
                    continue
                }
            }
            // Raw character: encode it back to UTF-8 bytes so the final
            // decode is uniform.
            out.write(c.toString().toByteArray(Charsets.UTF_8))
            i++
        }
        return String(out.toByteArray(), Charsets.UTF_8)
    }

    private fun hexVal(c: Char): Int = when (c) {
        in '0'..'9' -> c - '0'
        in 'a'..'f' -> c - 'a' + 10
        in 'A'..'F' -> c - 'A' + 10
        else -> -1
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
        val out = ByteArrayOutputStream()
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

    /** Base64 as produced for subscription bodies: tolerates whitespace,
     *  missing padding, the URL-safe alphabet and raw UTF-8 output. */
    fun b64decodeBytes(s0: String): ByteArray {
        var s = s0.trim().replace(Regex("\\s"), "")
        while (s.length % 4 != 0) s += "="
        val out = ByteArrayOutputStream()
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
        return out.toByteArray()
    }

    // ---- config builder ---------------------------------------------------

    /** Private ranges for the bypass-LAN rule (applied before the proxy). */
    private val PRIVATE_CIDRS = listOf(
        "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8",
        "169.254.0.0/16", "100.64.0.0/10", "224.0.0.0/4",
        "::1/128", "fe80::/10", "fc00::/7",
    )

    /** Tag of the resolver that goes through the proxy (dns.final). */
    internal const val DNS_REMOTE_TAG = "dns-remote"

    /** Tag of the resolver that dials directly. The proxy outbound resolves
     *  its own server name with this one, so bringing the tunnel up never
     *  depends on DNS that only exists once the tunnel is up. */
    internal const val DNS_DIRECT_TAG = "dns-direct"

    /** Full sing-box config for the profile's raw link: tun inbound, the
     *  parsed outbound, DNS and route. The optional settings carry the
     *  routing tab (bypass_lan, route_direct, route_block, remote_dns,
     *  log_level) — the same keys the desktop saves. */
    fun buildConfig(
        raw: String,
        name: String,
        settings: JSONObject? = null,
        cachePath: String? = null,
        rulesetPaths: Map<String, String> = emptyMap(),
    ): String {
        val s = settings ?: JSONObject()
        val outbound = buildOutbound(raw, name)

        val blockDomains = JSONArray()
        val blockIps = JSONArray()
        val blockSets = JSONArray()
        val directDomains = JSONArray()
        val directIps = JSONArray()
        val directSets = JSONArray()
        // tag → route.rule_set entry, filled only for the tags actually used,
        // so a plain domain/CIDR list still renders the same config as before.
        val setDefs = LinkedHashMap<String, JSONObject>()
        fun useSet(tag: String): String {
            setDefs.getOrPut(tag) { ruleSetDef(tag, rulesetPaths) }
            return tag
        }
        for (e in parseRouteList(s.optString("route_block"))) {
            when (e.kind) {
                RouteKind.CIDR -> blockIps.put(e.value)
                RouteKind.DOMAIN -> blockDomains.put(e.value)
                RouteKind.GEOIP, RouteKind.GEOSITE -> blockSets.put(useSet(e.value))
            }
        }
        for (e in parseRouteList(s.optString("route_direct"))) {
            when (e.kind) {
                RouteKind.CIDR -> directIps.put(e.value)
                RouteKind.DOMAIN -> directDomains.put(e.value)
                RouteKind.GEOIP, RouteKind.GEOSITE -> directSets.put(useSet(e.value))
            }
        }
        val hasBlock = blockDomains.length() > 0 || blockIps.length() > 0 || blockSets.length() > 0

        val rules = JSONArray()
            .put(JSONObject().put("action", "sniff"))
            .put(JSONObject().put("protocol", "dns").put("action", "hijack-dns"))
        if (hasBlock) {
            rules.put(JSONObject().put("outbound", "block").apply {
                if (blockDomains.length() > 0) put("domain_suffix", blockDomains)
                if (blockIps.length() > 0) put("ip_cidr", blockIps)
                if (blockSets.length() > 0) put("rule_set", blockSets)
            })
        }
        if (directDomains.length() > 0 || directIps.length() > 0 || directSets.length() > 0) {
            rules.put(JSONObject().put("outbound", "direct").apply {
                if (directDomains.length() > 0) put("domain_suffix", directDomains)
                if (directIps.length() > 0) put("ip_cidr", directIps)
                if (directSets.length() > 0) put("rule_set", directSets)
            })
        }
        if (s.optBoolean("bypass_lan", false)) {
            rules.put(
                JSONObject()
                    .put("ip_cidr", JSONArray().apply { PRIVATE_CIDRS.forEach { put(it) } })
                    .put("outbound", "direct"),
            )
        }

        // The proxy outbound resolves the SERVER's own domain through
        // dns-direct. With a single resolver tagged dns-remote (which itself
        // is routed through "proxy") a domain-based server could not be
        // looked up until the tunnel was already up: a deadlock the box
        // reported as an endless start retry.
        outbound.put("domain_resolver", DNS_DIRECT_TAG)

        val outbounds = JSONArray()
            .put(outbound)
            .put(JSONObject().put("type", "direct").put("tag", "direct"))
        if (hasBlock) {
            outbounds.put(JSONObject().put("type", "block").put("tag", "block"))
        }

        // TUN stack / strict route come from the settings screen (they were
        // hardcoded to "mixed"/false, so the UI toggles did nothing).
        val tunStack = s.optString("tun_stack", "mixed").let {
            if (it in setOf("mixed", "gvisor", "system")) it else "mixed"
        }
        val tunStrict = s.optBoolean("tun_strict_route", false)

        val cfg = JSONObject()
            .put(
                "log",
                JSONObject().put("level", s.optString("log_level").ifEmpty { "info" }).put("timestamp", true),
            )
            .put(
                "dns",
                JSONObject()
                    .put("servers", dnsServers(s))
                    .put("final", DNS_REMOTE_TAG)
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
                        .put("strict_route", tunStrict)
                        .put("stack", tunStack),
                ),
            )
            .put("outbounds", outbounds)
            .put(
                "route",
                JSONObject()
                    .put("auto_detect_interface", true)
                    .put("rules", rules)
                    .put("final", "proxy")
                    .apply {
                        if (setDefs.isNotEmpty()) {
                            put("rule_set", JSONArray().apply { setDefs.values.forEach { put(it) } })
                        }
                    },
            )
        // sing-box creates the cache file whenever a PlatformLogWriter is
        // set (box.go: PlatformLogWriter != nil → needCacheFile), and the
        // default path is relative to the process CWD — on Android that is
        // the read-only "/", so every start died with "open cache.db:
        // read-only file system". Point it at app-private storage.
        if (cachePath != null) {
            cfg.put(
                "experimental",
                JSONObject().put(
                    "cache_file",
                    JSONObject().put("enabled", true).put("path", cachePath),
                ),
            )
        }
        return cfg.toString()
    }

    /**
     * Two resolvers, deliberately: dns-remote is the user's (routed through
     * the proxy, the session's final resolver), dns-direct dials out
     * physically and exists so resolving the PROXY SERVER's own name never
     * depends on the tunnel. A single resolver plus `domain_resolver` on the
     * proxy outbound is the classic sing-box DNS loop.
     */
    internal fun dnsServers(s: JSONObject): JSONArray {
        val remote = parseDnsServer(s.optString("remote_dns"), DNS_REMOTE_TAG, detour = "proxy")
        val direct = JSONObject()
            .put("type", "udp")
            .put("tag", DNS_DIRECT_TAG)
            .put("server", "1.1.1.1")
            .put("detour", "direct")
        return JSONArray().put(remote).put(direct)
    }

    /**
     * "tcp://1.1.1.1" / "udp://8.8.8.8" / "tls://dns.example:853" /
     * "https://dns.example/dns-query" / bare address / "[::1]:53" →
     * the sing-box dns-server entry. Host and port stay separate fields:
     * the TLS family rejects a "host:port" string in `server`.
     */
    internal fun parseDnsServer(raw0: String, tag: String = DNS_REMOTE_TAG, detour: String? = null): JSONObject {
        val raw = raw0.trim()
        if (raw.isEmpty() || raw.equals("local", ignoreCase = true)) {
            // No remote resolver configured: 8.8.8.8 over UDP, proxied.
            return JSONObject()
                .put("type", "udp")
                .put("tag", tag)
                .put("server", "8.8.8.8")
                .apply { detour?.let { put("detour", it) } }
        }
        val withScheme = if (raw.contains("://")) raw else "udp://$raw"
        val scheme = withScheme.substringBefore("://").lowercase()
        if (scheme == "local") {
            return JSONObject()
                .put("type", "local")
                .put("tag", tag)
                .apply { detour?.let { put("detour", it) } }
        }
        val link = LinkParser.parse(withScheme)
        val type = when (scheme) {
            "https", "h3", "tls", "tcp", "udp", "quic" -> scheme
            else -> "udp"
        }
        val host = link?.host?.takeIf { it.isNotEmpty() } ?: "8.8.8.8"
        val out = JSONObject()
            .put("type", type)
            .put("tag", tag)
            .put("server", host)
        val port = link?.port ?: 0
        if (port in 1..65535) out.put("server_port", port)
        if (type == "https" || type == "h3") {
            val path = link?.path?.takeIf { it.isNotEmpty() } ?: "/dns-query"
            out.put("path", path)
        }
        if (detour != null) out.put("detour", detour)
        return out
    }

    /** One entry of a route list, classified by what it matches. */
    internal enum class RouteKind { DOMAIN, CIDR, GEOIP, GEOSITE }

    internal data class RouteEntry(val value: String, val kind: RouteKind)

    /**
     * Route lists: one entry per line (or comma/semicolon), # and //
     * comments stripped. Recognises the xray/v2rayN spellings geoip:xx and
     * geosite:xx, and normalises the "*.example.com" suffix form users bring
     * from there (sing-box wants the bare suffix: "example.com").
     *
     * The CIDR test is a real IPv4/IPv6 parse, not a character-class guess.
     * The old regex `^[0-9a-fA-F:.]+(/\\d{1,3})?$` matched every hex-only
     * domain — cafe.de, bbc.de, feed.ac — and emitted them as ip_cidr, which
     * sing-box refuses to start on ("invalid IP address").
     */
    internal fun parseRouteList(text: String): List<RouteEntry> {
        val out = mutableListOf<RouteEntry>()
        for (rawLine in text.split("\n", ",", ";")) {
            var line = rawLine.substringBefore("#").substringBefore("//").trim()
            if (line.isEmpty()) continue
            val lower = line.lowercase()
            when {
                lower.startsWith("geoip:") && lower.length > 6 ->
                    out.add(RouteEntry("geoip-" + lower.substring(6), RouteKind.GEOIP))
                lower.startsWith("geosite:") && lower.length > 8 ->
                    out.add(RouteEntry("geosite-" + lower.substring(8), RouteKind.GEOSITE))
                else -> {
                    if (line.startsWith("*.")) line = line.substring(2)
                    val value = line.lowercase()
                    out.add(RouteEntry(value, if (isCidr(value)) RouteKind.CIDR else RouteKind.DOMAIN))
                }
            }
        }
        return out
    }

    /** "1.2.3.4", "10.0.0.0/8", "2a14:e700::/124" — and nothing else. */
    internal fun isCidr(value: String): Boolean {
        val slash = value.indexOf('/')
        val addr = if (slash >= 0) value.substring(0, slash) else value
        if (addr.isEmpty()) return false
        val v4 = isIPv4(addr)
        val v6 = !v4 && isIPv6(addr)
        if (!v4 && !v6) return false
        if (slash < 0) return true
        val bits = value.substring(slash + 1)
        if (bits.isEmpty() || bits.length > 3 || !bits.all { it in '0'..'9' }) return false
        val n = bits.toIntOrNull() ?: return false
        return n in 0..(if (v4) 32 else 128)
    }

    internal fun isIPv4(s: String): Boolean {
        val parts = s.split('.')
        if (parts.size != 4) return false
        for (p in parts) {
            if (p.isEmpty() || p.length > 3) return false
            // Leading zeros are not a dotted-quad: "010.1.1.1" is invalid in
            // Go's net.ParseIP, and treating it as an IP here would produce a
            // rule sing-box cannot load.
            if (p.length > 1 && p[0] == '0') return false
            if (!p.all { it in '0'..'9' }) return false
            val v = p.toIntOrNull() ?: return false
            if (v !in 0..255) return false
        }
        return true
    }

    internal fun isIPv6(raw: String): Boolean {
        var s = raw
        if (s.startsWith("[") && s.endsWith("]")) s = s.substring(1, s.length - 1)
        if (s.isEmpty() || !s.contains(':')) return false
        if (s.contains('%')) return false // zone ids are not representable in a route
        val dbl = s.indexOf("::")
        if (dbl >= 0 && s.indexOf("::", dbl + 2) >= 0) return false
        fun groupOk(g: String): Boolean =
            g.isNotEmpty() && g.length <= 4 && g.all { it.isDigit() || it.lowercaseChar() in 'a'..'f' }

        val groups: List<String>
        if (dbl >= 0) {
            val head = s.substring(0, dbl)
            val tail = s.substring(dbl + 2)
            val hs = if (head.isEmpty()) emptyList() else head.split(':')
            val ts = if (tail.isEmpty()) emptyList() else tail.split(':')
            if (hs.size + ts.size > 7) return false
            groups = hs + ts
            if (groups.any { it.isEmpty() }) return false
            if (groups.any { !groupOk(it) && !(it == groups.last() && isIPv4(it)) }) return false
            return true
        }
        val all = s.split(':')
        if (all.size != 8) return false
        for ((i, g) in all.withIndex()) {
            if (g.isEmpty()) return false
            if (groupOk(g)) continue
            // ::ffff:1.2.3.4 form — only the last group may be IPv4
            if (i == 7 && isIPv4(g)) continue
            return false
        }
        return true
    }

    /** A remote rule-set definition. Bundled tags get initial_path, so the
     *  router has data before the first download; everything else downloads
     *  from jsDelivr (raw.githubusercontent is often unreachable from RU). */
    private fun ruleSetDef(tag: String, initial: Map<String, String>): JSONObject {
        val repo = if (tag.startsWith("geosite-")) "sing-geosite" else "sing-geoip"
        return JSONObject()
            .put("type", "remote")
            .put("tag", tag)
            .put("format", "binary")
            .put("url", "https://cdn.jsdelivr.net/gh/SagerNet/$repo@rule-set/$tag.srs")
            .apply { initial[tag]?.let { put("initial_path", it) } }
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

    private fun base(outboundType: String, uri: Link, fallbackPort: Int): JSONObject {
        val host = uri.host
        if (host.isEmpty()) throw IllegalArgumentException("нет адреса сервера")
        val port = if (uri.port in 1..65535) uri.port else fallbackPort
        return JSONObject()
            .put("type", outboundType)
            .put("tag", "proxy")
            .put("server", host)
            .put("server_port", port)
    }

    private fun tlsBlock(uri: Link, defaultName: String?, forced: Boolean): JSONObject? {
        val security = uri.param("security")?.lowercase()
        val sni = uri.param("sni") ?: defaultName
        val insecure = (uri.param("insecure") ?: uri.param("allowInsecure")
            ?: uri.param("allow_insecure")) in setOf("1", "true")
        val reality = security == "reality"
        if (!forced && security != "tls" && !reality) return null
        val tls = JSONObject().put("enabled", true)
        if (!sni.isNullOrBlank()) tls.put("server_name", sni)
        if (insecure) tls.put("insecure", true)
        val alpn = uri.param("alpn")
        if (!alpn.isNullOrBlank()) {
            tls.put("alpn", JSONArray().apply { alpn.split(",").forEach { if (it.isNotBlank()) put(it.trim()) } })
        }
        val fp = uri.param("fp")
        if (!fp.isNullOrBlank()) {
            tls.put("utls", JSONObject().put("enabled", true).put("fingerprint", fp))
        }
        if (reality) {
            tls.put(
                "reality",
                JSONObject()
                    .put("enabled", true)
                    .put("public_key", uri.param("pbk").orEmpty())
                    .put("short_id", uri.param("sid").orEmpty()),
            )
        }
        return tls
    }

    private fun transportBlock(uri: Link): JSONObject? {
        val net = uri.param("type")?.lowercase() ?: return null
        val path = uri.param("path")
        val host = uri.param("host")
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
                val sn = uri.param("serviceName")
                if (!sn.isNullOrBlank()) t.put("service_name", sn)
                t
            }
            else -> null // tcp / kcp / quic raw
        }
    }

    private fun vlessOutbound(raw: String, name: String): JSONObject {
        val uri = LinkParser.parse(raw) ?: throw IllegalArgumentException("vless: не разобрана ссылка")
        val o = base("vless", uri, 443)
            .put("uuid", uri.userInfo.takeIf { it.isNotEmpty() } ?: throw IllegalArgumentException("vless: нет uuid"))
            .put("packet_encoding", "xudp")
        uri.param("flow")?.takeIf { it.isNotBlank() }?.let { o.put("flow", it) }
        tlsBlock(uri, uri.host, false)?.let { o.put("tls", it) }
        transportBlock(uri)?.let { o.put("transport", it) }
        return o
    }

    private fun trojanOutbound(raw: String, name: String): JSONObject {
        val uri = LinkParser.parse(raw) ?: throw IllegalArgumentException("trojan: не разобрана ссылка")
        // userInfo is percent-decoded exactly once by LinkParser.
        val o = base("trojan", uri, 443)
            .put("password", uri.userInfo)
        tlsBlock(uri, uri.host, true)?.let { o.put("tls", it) }
        transportBlock(uri)?.let { o.put("transport", it) }
        return o
    }

    private fun hy2Outbound(raw: String, name: String): JSONObject {
        val uri = LinkParser.parse(raw) ?: throw IllegalArgumentException("hysteria2: не разобрана ссылка")
        // No URLDecoder here: userInfo is already percent-decoded, and a
        // second pass turned a literal '+' in a password into a space (and
        // decoded '%2B' into '+', then '+', then ' ' on reconnects).
        val o = base("hysteria2", uri, 443)
            .put("password", uri.userInfo)
        uri.param("obfs")?.takeIf { it.isNotBlank() }?.let {
            o.put(
                "obfs",
                JSONObject().put("type", it).put("password", uri.param("obfs-password").orEmpty()),
            )
        }
        val tls = tlsBlock(uri, uri.host, true) ?: JSONObject().put("enabled", true)
        if (!tls.has("alpn")) tls.put("alpn", JSONArray().put("h3"))
        o.put("tls", tls)
        return o
    }

    private fun tuicOutbound(raw: String, name: String): JSONObject {
        val uri = LinkParser.parse(raw) ?: throw IllegalArgumentException("tuic: не разобрана ссылка")
        val userinfo = uri.userInfo.takeIf { it.isNotEmpty() }
            ?: throw IllegalArgumentException("tuic: нет учётных данных")
        val o = base("tuic", uri, 443)
            .put("uuid", userinfo.substringBefore(":"))
        userinfo.substringAfter(":", missingDelimiterValue = "").takeIf { it.isNotEmpty() }?.let {
            o.put("password", it)
        }
        uri.param("congestion_control")?.takeIf { it.isNotBlank() }?.let { o.put("congestion_control", it) }
        uri.param("udp_relay_mode")?.takeIf { it.isNotBlank() }?.let { o.put("udp_relay_mode", it) }
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
        val uri = LinkParser.parse(raw)
        var body = rawSsBody(raw)
        val o = JSONObject().put("type", "shadowsocks").put("tag", "proxy")
        if (body.contains("@")) {
            val userinfo = body.substringBeforeLast("@")
            val hostPart = body.substringAfterLast("@")
            val cred = runCatching { b64decode(userinfo) }.getOrDefault(percentDecode(userinfo))
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
        uri?.param("plugin")?.takeIf { it.isNotBlank() }?.let { plugin ->
            val parts = plugin.split(";")
            o.put("plugin", parts[0])
            if (parts.size > 1) o.put("plugin_opts", parts.drop(1).joinToString(";"))
        }
        return o
    }
}

/**
 * A parsed share link. Deliberately small and Android-free: every field the
 * sing-box builders need, parsed exactly once, with one well-defined
 * percent-decoding step (and no '+' → space form decoding).
 */
internal class Link(
    val scheme: String,
    /** Percent-decoded, exactly once. Empty when the link has no userinfo. */
    val userInfo: String,
    /** Brackets stripped for IPv6 literals. */
    val host: String,
    /** 0 when absent or invalid. */
    val port: Int,
    /** Percent-decoded path ("/dns-query"), empty when absent. */
    val path: String,
    val fragment: String,
    private val query: Map<String, String>,
) {
    fun param(name: String): String? = query[name]
}

internal object LinkParser {

    private val SCHEME_RE = Regex("^[a-zA-Z][a-zA-Z0-9+.\\-]*$")

    fun parse(raw: String): Link? {
        val sep = raw.indexOf("://")
        if (sep <= 0) return null
        val scheme = raw.substring(0, sep)
        if (!SCHEME_RE.matches(scheme)) return null
        var rest = raw.substring(sep + 3)

        var fragment = ""
        val hash = rest.indexOf('#')
        if (hash >= 0) {
            fragment = SingboxLinks.percentDecode(rest.substring(hash + 1))
            rest = rest.substring(0, hash)
        }

        var queryRaw = ""
        val q = rest.indexOf('?')
        if (q >= 0) {
            queryRaw = rest.substring(q + 1)
            rest = rest.substring(0, q)
        }

        var path = ""
        val slash = rest.indexOf('/')
        if (slash >= 0) {
            path = SingboxLinks.percentDecode(rest.substring(slash))
            rest = rest.substring(0, slash)
        }

        // userinfo@host:port — lastIndexOf: a password may legally contain '@'
        var userInfo = ""
        var hostPort = rest
        val at = rest.lastIndexOf('@')
        if (at >= 0) {
            userInfo = SingboxLinks.percentDecode(rest.substring(0, at))
            hostPort = rest.substring(at + 1)
        }

        var host: String
        var port = 0
        if (hostPort.startsWith("[")) {
            val close = hostPort.indexOf(']')
            if (close < 0) return null
            host = hostPort.substring(1, close)
            val tail = hostPort.substring(close + 1)
            if (tail.startsWith(":")) port = tail.substring(1).toIntOrNull() ?: 0
        } else {
            val colons = hostPort.count { it == ':' }
            host = if (colons == 1) {
                val idx = hostPort.lastIndexOf(':')
                val p = hostPort.substring(idx + 1).toIntOrNull()
                if (p != null) {
                    port = p
                    hostPort.substring(0, idx)
                } else {
                    hostPort
                }
            } else {
                // No colon (hostname) or a bare IPv6 literal: no port.
                hostPort
            }
        }
        if (host.isEmpty()) return null
        if (port !in 0..65535) port = 0

        val query = LinkedHashMap<String, String>()
        if (queryRaw.isNotEmpty()) {
            for (pair in queryRaw.split('&')) {
                if (pair.isEmpty()) continue
                val eq = pair.indexOf('=')
                val k = if (eq >= 0) pair.substring(0, eq) else pair
                val v = if (eq >= 0) pair.substring(eq + 1) else ""
                query[SingboxLinks.percentDecode(k)] = SingboxLinks.percentDecode(v)
            }
        }

        return Link(
            scheme = scheme.lowercase(),
            userInfo = userInfo,
            host = host,
            port = port,
            path = path,
            fragment = fragment,
            query = query,
        )
    }
}
