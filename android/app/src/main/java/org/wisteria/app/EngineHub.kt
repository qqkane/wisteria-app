package org.wisteria.app

import org.json.JSONArray
import org.json.JSONObject
import java.util.ArrayDeque
import java.util.concurrent.CopyOnWriteArrayList

/**
 * Process-wide engine hub: the log ring, the diagnostics event feed and
 * the webview event registry. Both the VPN service (which receives the
 * gomobile callbacks) and the bridge (which serves them to the frontend)
 * live here, so history survives activity recreation while the engine is
 * running. The frontend pulls logs by seq and events by polling, the same
 * way it talks to the desktop backend.
 */
object EngineHub {

    class LogLine(val seq: Long, val tag: String, val text: String, val at: String)
    class DiagEvent(val at: String, val kind: String, val text: String)

    private val lock = Any()
    private val logs = ArrayDeque<LogLine>()
    private val diagEvents = ArrayDeque<DiagEvent>()
    private var nextSeq = 1L
    private const val MAX_LOGS = 2000
    private const val MAX_DIAG = 200

    @Volatile var profileId: String = ""
    @Volatile var profileName: String = ""
    @Volatile var profileType: String = ""

    /** Latest "health: rtt=… loss=… pings=…" slice from the engine. Those
     *  lines arrive every few seconds and used to flood the log ring (the
     *  whole Логи screen was them). They are consumed here instead: the
     *  frontend shows the freshest slice on its own live line above the
     *  logs, the ring keeps only actionable lines. Same shape as the
     *  desktop's core.CoreHealth. */
    @Volatile var lastHealth: JSONObject? = null
        private set

    private val HEALTH_RE = Regex("""health: rtt=(\d+)ms loss=(\d+)% pings=(\d+)""")

    /** Connect attempts that ended in a retry since the session started —
     *  the diagnostics page's «Попыток» chip (the desktop counts the same
     *  thing from the core's connect loop). */
    @Volatile var retryEvents: Int = 0
        private set

    /** Clears the diagnostics feed and counters; called when a new session
     *  starts so chips describe the current connection, not the previous. */
    fun resetDiag() {
        synchronized(lock) {
            diagEvents.clear()
            retryEvents = 0
        }
        lastHealth = null
    }

    /** (event, payloadJson) push targets; the activity registers its webview. */
    val emitters = CopyOnWriteArrayList<(String, String) -> Unit>()

    fun pushEvent(event: String, payload: JSONObject?) {
        val json = payload?.toString() ?: "null"
        emitters.forEach { it(event, json) }
    }

    fun pushEvent(event: String, payload: String) {
        emitters.forEach { it(event, payload) }
    }

    fun appLog(text: String) {
        addLog(LogLine(0, "app", text, AndroidStore.nowIso()))
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
            return // consumed: not a log line, no "log" event spam either
        }
        addLog(LogLine(0, "core", line, AndroidStore.nowIso()))
        classify(line)
        pushEvent("log", null)
    }

    private fun addLog(l: LogLine) {
        synchronized(lock) {
            val withSeq = LogLine(nextSeq++, l.tag, l.text, l.at)
            logs.addLast(withSeq)
            while (logs.size > MAX_LOGS) logs.removeFirst()
        }
    }

    fun logsSince(since: Long): JSONArray {
        val out = JSONArray()
        synchronized(lock) {
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

    /**
     * Maps engine log lines onto the diagnostics kinds the frontend knows
     * (the desktop classifies the core's stderr the same way). The engine
     * writes Russian lines, so the markers differ from the CLI's.
     */
    private fun classify(line: String) {
        val kind = when {
            line.contains("подключено к") || line.contains("туннель готов") -> "up"
            line.contains("handshake") -> "blocked"
            line.startsWith("dial:") || line.contains("resolve ") -> "nonet"
            line.contains("повтор через") -> "retry"
            else -> "info"
        }
        if (kind == "info") return
        synchronized(lock) {
            if (kind == "retry") retryEvents++
            diagEvents.addLast(DiagEvent(AndroidStore.nowIso(), kind, line))
            while (diagEvents.size > MAX_DIAG) diagEvents.removeFirst()
        }
    }

    /** Snapshot for the GetWisteriaDiag binding (the frontend shape). */
    fun diagEventsJson(): JSONArray {
        val out = JSONArray()
        synchronized(lock) {
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
    }
}
