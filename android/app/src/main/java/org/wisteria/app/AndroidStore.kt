package org.wisteria.app

import android.content.Context
import android.content.SharedPreferences
import org.json.JSONArray
import org.json.JSONObject
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone
import java.util.UUID

/**
 * JSON store in SharedPreferences with the same data shapes the desktop
 * serializes into profiles.json (identical Profile/Subscription field
 * names), so the shared frontend renders against identical objects.
 * Wisteria profiles additionally carry `servers` — the bundle's endpoint
 * list, passed through to the engine for failover.
 */
class AndroidStore(private val context: Context) {

    private val prefs: SharedPreferences
        get() = context.getSharedPreferences("wisteria", Context.MODE_PRIVATE)

    companion object {
        private val ISO = object : ThreadLocal<SimpleDateFormat>() {
            override fun initialValue() = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US).apply {
                timeZone = TimeZone.getTimeZone("UTC")
            }
        }
        fun nowIso(): String = ISO.get()!!.format(Date())
        fun epochIso(seconds: Long): String = ISO.get()!!.format(Date(seconds * 1000))
        fun newId(): String = UUID.randomUUID().toString().replace("-", "").take(16)

        val DEFAULT_SETTINGS = JSONObject(
            """{"language":"ru","theme":"lilac","local_port":2080,"http_port":2081,
               "sys_proxy":false,"bridge_enabled":false,"auto_connect":"",
               "wisteria_core":"","singbox_core":"","start_minimized":false,
               "confirm_exit":false,"check_ext_ip":true,"auto_ping_sec":0,
               "no_update_check":false,"listen_all":false,"bypass_lan":false,
               "remote_dns":"","log_level":"warn","mem_limit_mb":0,"mp_core":"",
               "core_ss":"sing-box","core_vmess":"sing-box","core_vless":"sing-box",
               "core_trojan":"sing-box","xray_core":"","route_direct":"",
               "route_block":"","tun_enabled":false,"tun_stack":"mixed",
               "tun_strict_route":false}"""
        )
    }

    private fun emptyStore() =
        JSONObject().put("profiles", JSONArray()).put("subs", JSONArray()).put("active", "")

    @Synchronized
    private fun load(): JSONObject {
        val raw = prefs.getString("store", null) ?: return emptyStore()
        return try {
            val o = JSONObject(raw)
            if (!o.has("profiles")) o.put("profiles", JSONArray())
            if (!o.has("subs")) o.put("subs", JSONArray())
            if (!o.has("active")) o.put("active", "")
            o
        } catch (_: Exception) {
            emptyStore()
        }
    }

    @Synchronized
    private fun save(store: JSONObject) {
        prefs.edit().putString("store", store.toString()).apply()
    }

    // ---- profiles ----

    @Synchronized
    fun listProfiles(): JSONArray {
        val arr = load().optJSONArray("profiles") ?: return JSONArray()
        // Copy element-wise: JSONArray(arr) would pick the JSONArray(Object)
        // constructor and throw "Not a primitive array".
        val out = JSONArray()
        for (i in 0 until arr.length()) out.put(arr.getJSONObject(i))
        return out
    }

    @Synchronized
    fun profile(id: String): JSONObject? {
        val arr = load().optJSONArray("profiles") ?: return null
        for (i in 0 until arr.length()) {
            val p = arr.getJSONObject(i)
            if (p.optString("id") == id) return p
        }
        return null
    }

    /** Adds the profile, assigning id/created_at; duplicate raw+type is a no-op. */
    @Synchronized
    fun addProfile(p: JSONObject): JSONObject? {
        val store = load()
        val arr = store.optJSONArray("profiles") ?: JSONArray()
        val raw = p.optString("raw")
        for (i in 0 until arr.length()) {
            val ex = arr.getJSONObject(i)
            if (raw.isNotEmpty() && ex.optString("raw") == raw && ex.optString("type") == p.optString("type")) {
                return null // already stored
            }
        }
        if (p.optString("id").isEmpty()) p.put("id", newId())
        if (p.optString("created_at").isEmpty()) p.put("created_at", nowIso())
        arr.put(p)
        store.put("profiles", arr)
        save(store)
        return p
    }

    @Synchronized
    fun deleteProfile(id: String): Boolean {
        val store = load()
        val arr = store.optJSONArray("profiles") ?: JSONArray()
        val out = JSONArray()
        var removed = false
        for (i in 0 until arr.length()) {
            val p = arr.getJSONObject(i)
            if (p.optString("id") == id) removed = true else out.put(p)
        }
        if (removed) {
            store.put("profiles", out)
            if (store.optString("active") == id) store.put("active", "")
            save(store)
        }
        return removed
    }

    @Synchronized
    fun updateProfile(p: JSONObject) {
        val store = load()
        val arr = store.optJSONArray("profiles") ?: return
        for (i in 0 until arr.length()) {
            if (arr.getJSONObject(i).optString("id") == p.optString("id")) {
                arr.put(i, p)
                break
            }
        }
        store.put("profiles", arr)
        save(store)
    }

    @Synchronized
    fun activeId(): String = load().optString("active")

    @Synchronized
    fun setActive(id: String) {
        val store = load()
        store.put("active", id)
        save(store)
    }

    // ---- subscriptions ----

    @Synchronized
    fun listSubs(): JSONArray {
        val arr = load().optJSONArray("subs") ?: return JSONArray()
        val out = JSONArray()
        for (i in 0 until arr.length()) out.put(arr.getJSONObject(i))
        return out
    }

    @Synchronized
    fun sub(id: String): JSONObject? {
        val arr = load().optJSONArray("subs") ?: return null
        for (i in 0 until arr.length()) {
            val s = arr.getJSONObject(i)
            if (s.optString("id") == id) return s
        }
        return null
    }

    @Synchronized
    fun addSub(url: String): JSONObject? {
        val store = load()
        val arr = store.optJSONArray("subs") ?: JSONArray()
        for (i in 0 until arr.length()) {
            if (arr.getJSONObject(i).optString("url") == url) return null
        }
        val s = JSONObject()
            .put("id", newId())
            .put("name", hostOf(url))
            .put("url", url)
            .put("count", 0)
            .put("updated_at", "")
            .put("error", "")
        arr.put(s)
        store.put("subs", arr)
        save(store)
        return s
    }

    @Synchronized
    fun updateSub(s: JSONObject) {
        val store = load()
        val arr = store.optJSONArray("subs") ?: return
        for (i in 0 until arr.length()) {
            if (arr.getJSONObject(i).optString("id") == s.optString("id")) {
                arr.put(i, s)
                break
            }
        }
        store.put("subs", arr)
        save(store)
    }

    /** Removes the subscription and exactly its own profiles. The bridge
     *  refuses the call while a profile of the sub is running; a merely
     *  selected profile is removed too, and a dangling active pointer is
     *  cleared (it used to survive and break the next auto-connect). */
    @Synchronized
    fun deleteSub(id: String) {
        val store = load()
        val profiles = store.optJSONArray("profiles") ?: JSONArray()
        val active = store.optString("active")
        val keep = JSONArray()
        var activeRemoved = false
        for (i in 0 until profiles.length()) {
            val p = profiles.getJSONObject(i)
            if (p.optString("sub_id") == id) {
                if (p.optString("id") == active) activeRemoved = true
            } else {
                keep.put(p)
            }
        }
        store.put("profiles", keep)
        if (activeRemoved) store.put("active", "")
        val subs = store.optJSONArray("subs") ?: JSONArray()
        val out = JSONArray()
        for (i in 0 until subs.length()) {
            if (subs.getJSONObject(i).optString("id") != id) out.put(subs.getJSONObject(i))
        }
        store.put("subs", out)
        save(store)
    }

    // ---- settings / autostart ----

    @Synchronized
    fun settings(): JSONObject {
        val raw = prefs.getString("settings", null) ?: return JSONObject(DEFAULT_SETTINGS.toString())
        val out = JSONObject(DEFAULT_SETTINGS.toString())
        return try {
            // manual shallow merge: org.json's JSONObject.merge is missing on
            // some Android compile SDKs
            val saved = JSONObject(raw)
            saved.keys().forEach { k -> out.put(k, saved.opt(k)) }
            out
        } catch (_: Exception) {
            out
        }
    }

    @Synchronized
    fun saveSettings(next: JSONObject) {
        prefs.edit().putString("settings", next.toString()).apply()
    }

    @Synchronized
    fun autostart(): Boolean = prefs.getBoolean("autostart", false)

    @Synchronized
    fun setAutostart(on: Boolean) {
        prefs.edit().putBoolean("autostart", on).apply()
    }

    private fun hostOf(url: String): String = try {
        java.net.URI(url).host ?: url
    } catch (_: Exception) {
        url
    }
}
