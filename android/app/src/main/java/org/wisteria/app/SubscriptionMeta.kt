package org.wisteria.app

import org.json.JSONObject

/**
 * Subscription response handling that does not need an Android Context, so
 * it can be unit-tested on the JVM. WisteriaBridge keeps the networking and
 * the profile merge; this object owns the two rules that used to be wrong:
 *
 *  - an HTML page (captive portal, expired token, 404) is not a link list;
 *  - Subscription-Userinfo / Profile-Title are standard response headers and
 *    are what every other client shows next to a subscription.
 *
 * Field names on the stored sub are additive ("userinfo" is new), so data
 * written by older builds stays valid.
 */
internal object SubscriptionMeta {

    /** True for an HTML page served instead of the link list: captive
     *  portals, expired-token pages and 404s all answer with markup, and
     *  parsing that yields zero profiles used to WIPE a working
     *  subscription. */
    fun looksLikeHtml(text: String): Boolean {
        val head = text.take(2048).lowercase()
        return "<!doctype" in head || "<html" in head || "<head" in head ||
            ("<body" in head && "</" in head)
    }

    /**
     * Subscription metadata from the response headers, the way v2rayN/SFA
     * read it:
     *   Subscription-Userinfo: upload=1; download=2; total=3; expire=1700000000
     *   Profile-Title: My sub   |   base64:<encoded title>
     */
    fun apply(sub: JSONObject, headers: Map<String, String>) {
        // Header names are case-insensitive; HttpURLConnection returns them
        // with the server's spelling.
        val lower = headers.entries.associate { it.key.lowercase() to it.value }
        lower["subscription-userinfo"]?.let { raw ->
            val o = JSONObject()
            for (part in raw.split(";", ",")) {
                val idx = part.indexOf('=')
                if (idx <= 0) continue
                val k = part.substring(0, idx).trim().lowercase()
                val v = part.substring(idx + 1).trim().toLongOrNull() ?: continue
                if (k in setOf("upload", "download", "total", "expire")) o.put(k, v)
            }
            if (o.length() > 0) sub.put("userinfo", o)
        }
        lower["profile-title"]?.let { raw ->
            var title = raw.trim()
            if (title.startsWith("base64:", ignoreCase = true)) {
                title = try {
                    String(SingboxLinks.b64decodeBytes(title.substring(7)), Charsets.UTF_8).trim()
                } catch (_: Throwable) {
                    ""
                }
            }
            if (title.isNotEmpty()) sub.put("name", title)
        }
    }
}
