package org.wisteria.app

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.util.ArrayDeque
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.Executors
import java.util.concurrent.ScheduledFuture
import java.util.concurrent.TimeUnit

/**
 * Process-wide engine hub: the log ring, the diagnostics event feed and
 * the webview event registry. Keeps logs and diagnostic events for 24 hours
 * and persists them to app-private storage so history survives app restarts.
 */
object EngineHub {

    class LogLine(val seq: Long, val tag: String, val text: String, val at: String, val tsMs: Long)
    class DiagEvent(val at: String, val kind: String, val text: String, val tsMs: Long)

    private val lock = Any()
    private val logs = ArrayDeque<LogLine>()
    private val diagEvents = ArrayDeque<DiagEvent>()
    private var nextSeq = 1L
    private const val MAX_LOGS = 4000
    private const val MAX_DIAG = 500
    private const val RETENTION_MS = 24L * 60L * 60L * 1000L

    @Volatile private var historyFile: File? = null
    @Volatile private var initialized = false
    private val saveExec = Executors.newSingleThreadScheduledExecutor {
        Thread(it, "wisteria-history").apply { isDaemon = true }
    }
    private var pendingSave: ScheduledFuture<*>? = null

    @Volatile var profileId: String = ""
    @Volatile var profileName: String = ""
    @Volatile var profileType: String = ""

    @Volatile var lastHealth: JSONObject? = null
        private set

    private val HEALTH_RE = Regex("""health: rtt=(\d+)ms loss=(\d+)% pings=(\d+)""")

    @Volatile var retryEvents: Int = 0
        private set

    /** Initializes 24-hour persistence once per process. Safe to call repeatedly. */
    fun init(context: Context) {
        if (initialized) return
        synchronized(lock) {
            if (initialized) return
            initialized = true
            val f = File(context.applicationContext.filesDir, "engine_history.json")
            historyFile = f
            loadFromDiskLocked(f)
        }
    }

    private fun pruneLocked(nowMs: Long = System.currentTimeMillis()) {
        val cutoff = nowMs - RETENTION_MS
        while (logs.isNotEmpty() && logs.first().tsMs < cutoff) {
            logs.removeFirst()
        }
        while (diagEvents.isNotEmpty() && diagEvents.first().tsMs < cutoff) {
            diagEvents.removeFirst()
        }
    }

    private fun loadFromDiskLocked(f: File) {
        if (!f.exists()) return
        try {
            val root = JSONObject(f.readText(Charsets.UTF_8))
            val nowMs = System.currentTimeMillis()
            val cutoff = nowMs - RETENTION_MS
            val logArr = root.optJSONArray("logs")
            if (logArr != null) {
                for (i in 0 until logArr.length()) {
                    val o = logArr.optJSONObject(i) ?: continue
                    val ts = o.optLong("ts", 0L)
                    if (ts < cutoff) continue
                    logs.addLast(
                        LogLine(
                            nextSeq++,
                            o.optString("tag", "app"),
                            o.optString("text", ""),
                            o.optString("at", ""),
                            ts,
                        )
                    )
                    while (logs.size > MAX_LOGS) logs.removeFirst()
                }
            }
            val diagArr = root.optJSONArray("diag")
            if (diagArr != null) {
                for (i in 0 until diagArr.length()) {
                    val o = diagArr.optJSONObject(i) ?: continue
                    val ts = o.optLong("ts", 0L)
                    if (ts < cutoff) continue
                    diagEvents.addLast(
                        DiagEvent(
                            o.optString("at", ""),
                            o.optString("kind", "info"),
                            o.optString("text", ""),
                            ts,
                        )
                    )
                    while (diagEvents.size > MAX_DIAG) diagEvents.removeFirst()
                }
            }
        } catch (_: Throwable) {
            // Corrupt history file: ignore and overwrite on next save.
        }
    }

    private fun scheduleSave() {
        val f = historyFile ?: return
        synchronized(saveExec) {
            pendingSave?.cancel(false)
            pendingSave = saveExec.schedule({ flushToDisk(f) }, 1500, TimeUnit.MILLISECONDS)
        }
    }

    private fun flushToDisk(f: File) {
        val payload = synchronized(lock) {
            pruneLocked()
            val logArr = JSONArray()
            for (l in logs) {
                logArr.put(
                    JSONObject()
                        .put("tag", l.tag)
                        .put("text", l.text)
                        .put("at", l.at)
                        .put("ts", l.tsMs)
                )
            }
            val diagArr = JSONArray()
            for (e in diagEvents) {
                diagArr.put(
                    JSONObject()
                        .put("at", e.at)
                        .put("kind", e.kind)
                        .put("text", e.text)
                        .put("ts", e.tsMs)
                )
            }
            JSONObject().put("logs", logArr).put("diag", diagArr).toString()
        }
        try {
            val tmp = File(f.parentFile, f.name + ".tmp")
            tmp.writeText(payload, Charsets.UTF_8)
            tmp.renameTo(f)
        } catch (_: Throwable) {
        }
    }

    /** Resets per-session health/retry counters while preserving 24h diagnostics history. */
    fun resetDiag() {
        synchronized(lock) {
            pruneLocked()
            retryEvents = 0
        }
        lastHealth = null
    }

    val emitters = CopyOnWriteArrayList<(String, String) -> Unit>()

    fun pushEvent(event: String, payload: JSONObject?) {
        val json = payload?.toString() ?: "null"
        emitters.forEach { it(event, json) }
    }

    fun pushEvent(event: String, payload: String) {
        emitters.forEach { it(event, payload) }
    }

    fun appLog(text: String) {
        val now = System.currentTimeMillis()
        addLog(LogLine(0, "app", text, AndroidStore.nowIso(), now))
        pushEvent("log", null)
    }

    fun coreLog(line: String) {
        val m = HEALTH_RE.find(line)
        if (m != null) {
            val (rtt, loss, pings) = m.destructured
            lastHealth = JSONObject()
                .put("active", true)
                .put("rtt_ms", rtt.toLong())
                .put("loss_pct", loss.toLong())
                .put("pings", pings.toLong())
                .put("at", AndroidStore.nowIso())
            return
        }
        val now = System.currentTimeMillis()
        addLog(LogLine(0, "core", line, AndroidStore.nowIso(), now))
        classify(line, now)
        pushEvent("log", null)
    }

    private fun addLog(l: LogLine) {
        synchronized(lock) {
            pruneLocked(l.tsMs)
            val withSeq = LogLine(nextSeq++, l.tag, l.text, l.at, l.tsMs)
            logs.addLast(withSeq)
            while (logs.size > MAX_LOGS) logs.removeFirst()
        }
        scheduleSave()
    }

    fun logsSince(since: Long): JSONArray {
        val out = JSONArray()
        synchronized(lock) {
            pruneLocked()
            for (l in logs) {
                if (l.seq > since) {
                    out.put(
                        JSONObject()
                            .put("seq", l.seq)
                            .put("tag", l.tag)
                            .put("text", l.text)
                            .put("at", l.at)
                    )
                }
            }
        }
        return out
    }

    private fun classify(line: String, nowMs: Long) {
        val kind = when {
            line.contains("подключено к") || line.contains("туннель готов") -> "up"
            line.contains("handshake") -> "blocked"
            line.startsWith("dial:") || line.contains("resolve ") -> "nonet"
            line.contains("повтор через") -> "retry"
            else -> "info"
        }
        if (kind == "info") return
        synchronized(lock) {
            pruneLocked(nowMs)
            if (kind == "retry") retryEvents++
            diagEvents.addLast(DiagEvent(AndroidStore.nowIso(), kind, line, nowMs))
            while (diagEvents.size > MAX_DIAG) diagEvents.removeFirst()
        }
        scheduleSave()
    }

    fun diagEventsJson(): JSONArray {
        val out = JSONArray()
        synchronized(lock) {
            pruneLocked()
            for (e in diagEvents) {
                out.put(
                    JSONObject()
                        .put("at", e.at)
                        .put("kind", e.kind)
                        .put("text", e.text)
                        .put("raw", e.text)
                )
            }
        }
        return out
    }

    fun clearLogs() {
        synchronized(lock) {
            logs.clear()
            diagEvents.clear()
            retryEvents = 0
        }
        historyFile?.let { f ->
            try { f.delete() } catch (_: Throwable) {}
        }
    }
}
