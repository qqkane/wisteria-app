package org.wisteria.app

import org.json.JSONArray
import org.json.JSONObject

/**
 * TLS transport (wisteria wave 4): the endpoint may sit behind a real TLS
 * listener (server crypto/tls, client uTLS Firefox_120). Mirrors the rules
 * of the protocol's internal/config loader and the desktop shell's
 * internal/core/profile.go so a profile behaves identically on every
 * client. Zero values mean the bare-TCP default: profiles saved before
 * wave 4 have no keys and stay tcp — backward compatible by construction.
 *
 * Profile JSON keys (same as the desktop store): "transport" (absent/"tcp"
 * normalized onto absent, or "tls"), "tls_sni" (optional ServerName
 * override; absent = endpoint host), "tls_insecure" (bool, lab only).
 * The bundle server rows inside "servers" carry the same three keys —
 * mobile.ParseLinks emits them only for links that set TLS, and the engine
 * validates them again at Start.
 */
object TlsTransport {

    /** Whether s is a transport value this build speaks: "" (the bare-TCP
     *  default), "tcp" or "tls". Case-sensitive like the core's config
     *  loader — a typo'd transport must not silently fall back to tcp and
     *  change what lands on the wire. Returns the error text or null. */
    fun validateTransport(s: String): String? = when (s) {
        "", "tcp", "tls" -> null
        else -> "transport должен быть \"tcp\" или \"tls\", получено \"$s\""
    }

    /** Whether s is empty or a sane TLS ServerName: RFC 1123 host labels
     *  (letters/digits/hyphens), each 1..63 chars, no underscores, no empty
     *  labels, at most 253 chars total. An IP literal is rejected on
     *  purpose: RFC 6066 forbids literal addresses in the ServerName
     *  extension, and name verification against a DNS-san certificate
     *  would fail regardless — an endpoint that can only work by IP
     *  belongs to tls_insecure territory. Returns the error text or null.
     *  Error strings never carry credentials — only the SNI value itself. */
    fun validSni(s: String): String? {
        if (s.isEmpty()) return null
        if (s.length > 253) return "tls_sni \"$s\" длиннее 253 символов"
        if (isIpLiteral(s)) {
            return "tls_sni \"$s\" — IP-адрес: нужен домен (или уберите tls_sni — по умолчанию берётся хост endpoint)"
        }
        s.split(".").forEachIndexed { i, label ->
            if (label.isEmpty()) return "tls_sni \"$s\": пустая метка (позиция ${i + 1})"
            if (label.length > 63) return "tls_sni \"$s\": метка \"$label\" длиннее 63 символов"
            for (r in label) {
                if (!(r in 'a'..'z' || r in 'A'..'Z' || r in '0'..'9' || r == '-')) {
                    return "tls_sni \"$s\": символ \"$r\" недопустим в имени хоста"
                }
            }
            if (label.first() == '-' || label.last() == '-') {
                return "tls_sni \"$s\": метка \"$label\" не должна начинаться или заканчиваться дефисом"
            }
        }
        return null
    }

    // IPv4 dotted quad (each octet 0..255) or anything with a colon
    // (an IPv6 literal or garbage — both belong to the IP rejection with
    // its clearer message than the generic character check).
    private val IPV4 = Regex("""^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$""")

    private fun isIpLiteral(s: String): Boolean {
        if (s.contains(':')) return true
        val m = IPV4.find(s) ?: return false
        return m.groupValues.drop(1).all { it.toInt() in 0..255 }
    }

    /** Canonicalizes and validates the TLS fields of a wisteria profile in
     *  place: "tcp" collapses onto the absent default (one canonical form
     *  in the store, like the desktop), the SNI is trimmed, and
     *  tls_sni/tls_insecure without transport = "tls" are rejected — the
     *  same combo rule the link parsers and the core's config loader
     *  apply. Throws IllegalArgumentException with a user-facing message;
     *  every save path funnels through AndroidStore.addProfile, so a
     *  manual-form or hand-edited-store profile is covered even though it
     *  skipped the parsers. Non-wisteria types are untouched. */
    fun normalizeTls(p: JSONObject) {
        if (p.optString("type") != "wisteria") return
        val tr = p.optString("transport").trim()
        when (tr) {
            "", "tcp" -> p.remove("transport")
            "tls" -> p.put("transport", "tls")
            else -> throw IllegalArgumentException(validateTransport(tr))
        }
        val sni = p.optString("tls_sni").trim()
        if (sni.isEmpty()) p.remove("tls_sni") else p.put("tls_sni", sni)
        validSni(sni)?.let { throw IllegalArgumentException(it) }
        if (tr != "tls" && (sni.isNotEmpty() || p.optBoolean("tls_insecure"))) {
            throw IllegalArgumentException("tls_sni/tls_insecure требуют transport = \"tls\"")
        }
    }

    /** Bundle rows carry the per-server TLS keys; the profile's own fields
     *  mirror the primary row (servers[0] — the same row the profile's
     *  host/port come from) so the UI badge and the store validation see
     *  them. Rows without TLS leave the profile bare. mobile.ParseLinks
     *  has already validated the rows at parse time. */
    fun applyPrimaryTls(p: JSONObject, servers: JSONArray) {
        if (servers.length() == 0) return
        val row = servers.getJSONObject(0)
        if (row.optString("transport") != "tls") return
        p.put("transport", "tls")
        row.optString("tls_sni").takeIf { it.isNotEmpty() }?.let { p.put("tls_sni", it) }
        if (row.optBoolean("tls_insecure")) p.put("tls_insecure", true)
    }

    /** The dial rows for the engine's MobileConfig. Profiles imported from
     *  bundle links store the whole server list — it passes through as is
     *  (the rows already carry their TLS keys). A manually entered profile
     *  has no list: build the single row from host:port plus the pinned
     *  pub and the profile's TLS fields, omitting the TLS keys unless set
     *  (omitempty semantics — a bare-tcp row stays byte-identical to the
     *  pre-wave-4 shape). The engine rejects a config with neither a
     *  server list nor an endpoint, so the manual row is required, not
     *  cosmetic. */
    fun engineServers(p: JSONObject): JSONArray {
        val stored = p.optJSONArray("servers")
        if (stored != null && stored.length() > 0) return stored
        val host = p.optString("host")
        val port = p.optInt("port", 0)
        val endpoint = if (port > 0) "$host:$port" else host
        val row = JSONObject().put("endpoint", endpoint)
        p.optString("pub").takeIf { it.isNotEmpty() }?.let { row.put("pub", it) }
        p.optString("transport").takeIf { it.isNotEmpty() }?.let { row.put("transport", it) }
        p.optString("tls_sni").takeIf { it.isNotEmpty() }?.let { row.put("tls_sni", it) }
        if (p.optBoolean("tls_insecure")) row.put("tls_insecure", true)
        return JSONArray().put(row)
    }
}
