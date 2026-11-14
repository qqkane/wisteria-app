package org.wisteria.app

import android.content.Context
import android.net.IpPrefix
import androidx.annotation.RequiresApi
import java.net.InetAddress

/**
 * RU networks for "do not proxy Russian sites" (the Android counterpart of
 * the desktop routing.mode=bypass_ru).
 *
 * The list is a copy of wisteria-go/internal/geo/lists/ru_networks.txt — the
 * exact CIDRs the desktop core routes direct. It ships as an asset because
 * the Android build consumes the engine as a prebuilt AAR and cannot read the
 * Go module's embedded table; refreshing means copying that file again.
 *
 * VpnService has no notion of domains, so this mode is IP-only: a .ru site
 * hosted behind a foreign CDN stays inside the tunnel. The desktop, which
 * also matches domain suffixes, is more precise.
 *
 * IPv6 is skipped on purpose: the wisteria engine relays IPv4 only and
 * blackholes v6 inside the TUN (see startWisteria's ::/0 route). Excluding v6
 * prefixes would let RU v6 traffic out directly, which is a separate
 * decision — not a silent side effect of this one.
 */
object RuRoutes {
    private const val ASSET = "ru_networks.txt"

    @Volatile
    private var cache: Array<IpPrefix>? = null

    /**
     * Parsed IPv4 prefixes, cached for the process lifetime: parsing ~8.7k
     * CIDRs takes tens of milliseconds and every connect would otherwise pay
     * it again. Requires API 33 (IpPrefix/excludeRoute).
     */
    @RequiresApi(33)
    fun ipv4(context: Context): Array<IpPrefix> {
        cache?.let { return it }
        synchronized(this) {
            cache?.let { return it }
            val out = ArrayList<IpPrefix>(9000)
            context.assets.open(ASSET).bufferedReader().useLines { lines ->
                for (raw in lines) {
                    val line = raw.trim()
                    if (line.isEmpty() || line.startsWith("#")) continue
                    val slash = line.indexOf('/')
                    if (slash <= 0) continue
                    // Numeric parse only: InetAddress.getByName would treat a
                    // malformed entry as a hostname and block the connect on a
                    // DNS lookup. The asset is trusted, but a botched refresh
                    // must not freeze the VPN.
                    val bytes = parseIPv4(line.substring(0, slash)) ?: continue
                    val bits = line.substring(slash + 1).toIntOrNull() ?: continue
                    if (bits !in 0..32) continue
                    out.add(IpPrefix(InetAddress.getByAddress(bytes), bits))
                }
            }
            return out.toTypedArray().also { cache = it }
        }
    }

    /**
     * User-supplied IPv4 routes ("203.0.113.0/24" or a bare address), one per
     * line, comma or semicolon; '#' starts a comment. Only IPv4: the engine
     * relays IPv4, and an excluded v6 prefix would silently un-tunnel it.
     * Requires API 33 (IpPrefix).
     */
    @RequiresApi(33)
    fun parseList(text: String): List<IpPrefix> {
        val out = ArrayList<IpPrefix>()
        for (raw in text.split('\n', ',', ';')) {
            val line = raw.substringBefore('#').trim()
            if (line.isEmpty()) continue
            val slash = line.indexOf('/')
            val addr = if (slash > 0) line.substring(0, slash) else line
            val bytes = parseIPv4(addr.trim()) ?: continue
            val bits = if (slash > 0) line.substring(slash + 1).trim().toIntOrNull() ?: continue else 32
            if (bits !in 0..32) continue
            out.add(IpPrefix(InetAddress.getByAddress(bytes), bits))
        }
        return out
    }

    /** Dotted-quad to 4 bytes, or null when the string is not a plain IPv4. */
    private fun parseIPv4(s: String): ByteArray? {
        val parts = s.split('.')
        if (parts.size != 4) return null
        val out = ByteArray(4)
        for (i in 0..3) {
            val p = parts[i]
            if (p.isEmpty() || p.length > 3) return null
            var v = 0
            for (c in p) {
                if (c < '0' || c > '9') return null
                v = v * 10 + (c - '0')
            }
            if (v > 255) return null
            out[i] = v.toByte()
        }
        return out
    }
}
