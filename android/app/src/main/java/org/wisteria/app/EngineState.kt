package org.wisteria.app

import android.content.Context
import android.content.SharedPreferences
import org.json.JSONObject
import java.util.concurrent.CopyOnWriteArraySet

/**
 * Bridge between the Go engine (gomobile callbacks arrive on non-main
 * threads), the VPN service (notification updates) and the activity
 * (UI). Holds the current state, the ring of recent log lines and the
 * stored profile. UI listeners must hop to the main thread themselves.
 */
object EngineState {

    interface Listener {
        fun onChanged(state: String)
        fun onLog(line: String)
    }

    @Volatile
    var current: String = "idle"
        private set

    private const val PREFS = "wisteria"
    private const val PREF_PROFILE = "profile"
    private const val MAX_LOG_LINES = 500

    private val logs = ArrayDeque<String>()
    private val listeners = CopyOnWriteArraySet<Listener>()

    fun set(state: String) {
        current = state
        listeners.forEach { it.onChanged(state) }
    }

    @Synchronized
    fun addLog(line: String) {
        logs.addLast(line)
        while (logs.size > MAX_LOG_LINES) logs.removeFirst()
        listeners.forEach { it.onLog(line) }
    }

    @Synchronized
    fun logText(): String = logs.joinToString("\n")

    fun addListener(l: Listener) {
        listeners.add(l)
    }

    fun removeListener(l: Listener) {
        listeners.remove(l)
    }

    private fun prefs(context: Context): SharedPreferences =
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    /**
     * The stored profile: a single parsed link entry as JSON
     * {name, key, servers:[{endpoint, pub}]}. Invites without a key
     * are rejected at import time, so the key is always present.
     */
    fun profile(context: Context): JSONObject? =
        prefs(context).getString(PREF_PROFILE, null)?.let {
            try {
                JSONObject(it)
            } catch (_: Exception) {
                null
            }
        }

    fun saveProfile(context: Context, p: JSONObject) {
        prefs(context).edit().putString(PREF_PROFILE, p.toString()).apply()
    }
}
