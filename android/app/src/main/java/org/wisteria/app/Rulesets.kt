package org.wisteria.app

import android.content.Context
import java.io.File

/**
 * Bundled sing-box rule-sets (.srs) backing the geoip:/geosite: tokens in the
 * routing fields.
 *
 * sing-box downloads remote rule-sets and caches them in cache.db, but the
 * very first start would then depend on GitHub/jsDelivr being reachable —
 * exactly the moment a user is setting up a tunnel because something is
 * blocked. The six most likely sets (RU and its neighbours' networks, plus
 * the ad list) ship inside the APK instead: they are copied to app-private
 * storage once and handed to sing-box as `initial_path`, so the router has
 * real data before any network request. Anything else still downloads
 * normally.
 *
 * Refresh: replace the .srs files in assets/rulesets/ and bump VERSION — the
 * marker file makes the next start re-extract them.
 */
object Rulesets {
    private const val ASSET_DIR = "rulesets"
    private const val VERSION = "1"

    private val TAGS = listOf(
        "geoip-ru",
        "geoip-cn",
        "geoip-kz",
        "geoip-by",
        "geoip-ua",
        "geosite-category-ads-all",
    )

    @Volatile
    private var cached: Map<String, String>? = null

    /** tag → absolute path of the extracted .srs, for `initial_path`. */
    fun paths(context: Context): Map<String, String> {
        cached?.let { return it }
        synchronized(this) {
            cached?.let { return it }
            val dir = File(context.filesDir, ASSET_DIR).apply { mkdirs() }
            val marker = File(dir, ".version")
            val fresh = runCatching { marker.readText().trim() }.getOrNull() != VERSION
            val out = LinkedHashMap<String, String>()
            for (tag in TAGS) {
                val dst = File(dir, "$tag.srs")
                try {
                    if (fresh || !dst.exists()) {
                        context.assets.open("$ASSET_DIR/$tag.srs").use { input ->
                            dst.outputStream().use { input.copyTo(it) }
                        }
                    }
                    out[tag] = dst.absolutePath
                } catch (e: Exception) {
                    // A missing asset must not take the tunnel down: the tag
                    // simply falls back to a remote download.
                    android.util.Log.w("Wisteria", "ruleset $tag: ${e.message}")
                }
            }
            if (fresh) runCatching { marker.writeText(VERSION) }
            return out.also { cached = it }
        }
    }
}
