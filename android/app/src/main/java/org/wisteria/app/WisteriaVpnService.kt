package org.wisteria.app

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import org.json.JSONArray
import org.json.JSONObject
import org.wisteria.mobile.Callback
import org.wisteria.mobile.Mobile
import java.util.concurrent.Executors
import java.util.concurrent.ScheduledFuture
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import kotlin.math.min

/**
 * VPN frontend for the two embedded Go engines. The wisteria engine gets
 * its tun fd upfront (Builder.establish() → Mobile.start); the sing-box
 * engine (vless/vmess/trojan/ss/hysteria2/tuic) asks for it itself through
 * Callback.onOpenTun once the box core boots. Both engines push
 * state/log events through EngineHub, which the webview bridge serves to
 * the shared frontend.
 *
 * The service tears everything down: the fds stay owned here, Go never
 * closes them. Every socket the engines open is bound to the VPN
 * interface unless the Protect callback allows it out — that callback is
 * VpnService.protect().
 */
class WisteriaVpnService : VpnService() {

    companion object {
        const val ACTION_CONNECT = "org.wisteria.app.CONNECT"
        const val ACTION_DISCONNECT = "org.wisteria.app.DISCONNECT"
        const val ACTION_RECONNECT = "org.wisteria.app.RECONNECT"
        const val EXTRA_PROFILE_ID = "profile_id"
        private const val CHANNEL_ID = "vpn"
        private const val NOTIF_ID = 1
        private const val TUN_ADDRESS = "10.8.0.2"
        private const val TUN_PREFIX = 24
        private const val TUN_MTU = 1400
        private const val DNS = "8.8.8.8"
    }

    // @Volatile: teardown() runs on the disconnect thread (or onDestroy)
    // and reads these under stateLock, while the connect() executor writes
    // them — a plain field write has no happens-before edge with the
    // synchronized reader, so a racing teardown could see a stale null and
    // leak the fd (or close a fresh one). boxTun is assigned under
    // stateLock in onOpenTun; the volatile belt covers both.
    @Volatile
    private var tun: ParcelFileDescriptor? = null
    @Volatile
    private var boxTun: ParcelFileDescriptor? = null
    private var boxNetCallback: ConnectivityManager.NetworkCallback? = null

    // Native-engine counterpart of the box interface monitor. The wisteria
    // engine dials once and never re-reads the OS routing table, so a
    // Wi-Fi ⇄ cellular switch leaves its sockets bound to a dead network
    // (the tunnel stays "connected" over black hole traffic) until the
    // health watchdog times out. Watching ConnectivityManager ourselves and
    // rebuilding the session on a real default-network change closes that
    // gap; lastActiveNetHandle makes the callback deliver-on-register and
    // spurious same-network events no-ops.
    private var wisteriaNetCallback: ConnectivityManager.NetworkCallback? = null
    @Volatile
    private var lastActiveNetHandle: Long? = null
    @Volatile
    private var netSwitchFuture: ScheduledFuture<*>? = null
    private val netSwitchLock = Any()

    private val executor = Executors.newSingleThreadExecutor()
    private var engineIsBox = false

    /** Serializes an engine start against an engine stop across threads.
     *  Disconnect tears down on its own thread (on purpose — it must not
     *  queue behind a blocking start), so teardown() no longer holds
     *  stateLock while it stops the engine. Without this lock a Connect
     *  that passed the stateLock guard while such a stop was mid-flight
     *  could start the next engine and then have the old Mobile.stop()
     *  cancel the new session's context. */
    private val engineLock = Any()

    /** True while a connect() is between its entry guard and the engine
     *  flags being set; guarded by stateLock. */
    private var starting = false

    /** Guards tun/boxTun/engineIsBox/teardownEpoch: CancelConnect tears
     *  down on its own thread even while connect() is stuck inside a
     *  blocking native engine call. */
    private val stateLock = Any()

    /** Bumped by every teardown. connect() captures it before starting an
     *  engine and re-checks afterwards: a Disconnect that raced the start
     *  used to be lost entirely (teardown saw nothing running yet, the
     *  engine then came up anyway, and the UI said "disconnected" over a
     *  live tunnel). */
    private var teardownEpoch = 0L

    /** Unix seconds of the current session start, echoed into every pushed
     *  state event. The desktop pushes the full core.Status (with
     *  StartedAt); without it the frontend's uptime card never left "—"
     *  after connect: GetState does carry started_at, but the 3 s poll only
     *  re-renders when the state word differs. Cleared in teardown(). */
    @Volatile
    private var sessionStartEpoch = 0L

    /** Bumped by every user-initiated Disconnect. A CONNECT captures this
     *  generation in onStartCommand — before it is queued behind anything —
     *  and connect() refuses to start the engine if it changed meanwhile:
     *  a queued connect must not override a Cancel that landed while it
     *  sat in the queue. (A plain userStopped check cannot tell "cancel
     *  raced this connect" from "initial state" — the field starts true.) */
    @Volatile
    private var cancelEpoch = 0L

    // ---- app-side reconnect parity with the desktop (v1.1.6 core fix) ----
    // The desktop shell watches the SOCKS port every 2s and auto-reconnects
    // with backoff when the session dies without the user asking. Android
    // has no SOCKS port: the liveness probe is the engine's own status
    // word, polled on the same 2s cycle — if the service still owns a tun
    // while the engine says idle/disconnected, the engine died on its own.
    // Reconnect runs with backoff 1→30s; only a user-initiated Disconnect
    // (or a VPN consent revoke) cancels it.

    private val watchdogExec = Executors.newSingleThreadScheduledExecutor()
    private var watchdogFuture: ScheduledFuture<*>? = null
    private val backoffSecs = intArrayOf(1, 2, 4, 8, 16, 30)
    private var reconnectStep = 0

    /** One reconnect may be in the queue at a time. Every teardown ends
     *  with the engine's own onState("disconnected") callback (Stop pushes
     *  the state), and without this gate each reconnect cycle queued the
     *  next one: teardown → stop → "disconnected" → schedule → teardown…
     *  an infinite flap loop that killed each freshly built session about
     *  a second after it connected. Cleared on the next "connecting"/
     *  "connected" so a later drop can queue again. */
    private val reconnectQueued = java.util.concurrent.atomic.AtomicBoolean(false)

    /** Depth of nested/overlapping teardown() calls. A plain boolean was
     *  cleared by whichever pass finished first, so a Disconnect racing
     *  onDestroy() or an internal reconnect re-armed the "the engine died
     *  on its own" path and queued a spurious scheduleReconnect(). */
    private val tearingDownDepth = AtomicInteger(0)
    private val isTearingDown: Boolean get() = tearingDownDepth.get() > 0

    /** True while no session is desired: user stop, revoke, startup. The
     *  watchdog and the reconnect queue must stay quiet while it is set. */
    @Volatile
    private var userStopped = true

    private fun startWatchdog() {
        stopWatchdog()
        watchdogFuture = watchdogExec.scheduleWithFixedDelay({
            try {
                if (!userStopped) {
                    val st = JSONObject(Mobile.status()).optString("state")
                    val running = synchronized(stateLock) { tun != null || engineIsBox }
                    if (running && (st == "disconnected" || st == "idle")) {
                        EngineHub.appLog("watchdog: движок упал (state=$st) — автопереподключение")
                        scheduleReconnect()
                    }
                }
            } catch (_: Throwable) {
            }
        }, 2000L, 2000L, TimeUnit.MILLISECONDS)
    }

    private fun stopWatchdog() {
        watchdogFuture?.cancel(false)
        watchdogFuture = null
    }

    /** Serialized teardown of the dead session + a delayed connect of the
     *  active profile. connect("") re-reads the active id, so a profile
     *  renamed/switched while offline reconnects to the current one. */
    private fun scheduleReconnect() {
        if (userStopped) return
        if (!reconnectQueued.compareAndSet(false, true)) return
        val delay = backoffSecs[min(reconnectStep, backoffSecs.size - 1)]
        reconnectStep++
        // Generation this reconnect attempt belongs to, sampled when the
        // backoff is scheduled: a Cancel (or a fresh Connect, which also
        // bumps the epoch) that lands during the delay must abort this
        // attempt instead of letting it fire with the newest generation.
        val epoch = cancelEpoch
        EngineHub.appLog("автопереподключение через ${delay}с (попытка $reconnectStep)")
        watchdogExec.schedule({
            if (userStopped) { reconnectQueued.set(false); return@schedule }
            executor.execute {
                if (userStopped) { reconnectQueued.set(false); return@execute }
                if (cancelEpoch != epoch) { reconnectQueued.set(false); return@execute }
                teardown()
                // Internal relaunch: nobody called startForegroundService
                // for it, so a backgrounded app must not crash the retry.
                connect("", epoch, fgsRequired = false)
            }
        }, delay.toLong(), TimeUnit.SECONDS)
    }

    private val callback = object : Callback {
        override fun onLog(line: String?) {
            // Every callback crosses JNI into Go: an exception escaping
            // here becomes a Go panic and aborts the whole process. The
            // bodies below are already defensive, this is the belt that
            // guarantees a callback can never kill the app.
            try {
                EngineHub.coreLog(line ?: "")
            } catch (t: Throwable) {
                android.util.Log.w("WisteriaVpn", "onLog", t)
            }
        }

        override fun onState(state: String?) {
            try {
                // A rejected handshake (wrong/rotated key, server-side peer
                // removed) is terminal: the engine cannot fix it by retrying,
                // and scheduleReconnect() would hammer the server every 30 s
                // forever. Stand down and tell the user what to do — only a
                // fresh Connect (a new link) clears this.
                if (state == "rejected") {
                    userStopped = true
                    EngineHub.pushEvent(
                        "state",
                        statusJson("disconnected", "Сервер отклонил рукопожатие — проверьте ключ или обновите ссылку"),
                    )
                    // Post the final word first: teardown() removes the
                    // foreground notification, and re-posting it afterwards
                    // would leave an undismissable "Отключено" ghost.
                    updateNotification("disconnected")
                    WisteriaTileService.refresh(this@WisteriaVpnService)
                    teardown(pushState = false)
                    stopSelf()
                    return
                }
                val mapped = when (state) {
                    "connecting", "retrying" -> "connecting"
                    "connected" -> "connected"
                    // The engine is alive but has no usable network yet and
                    // keeps retrying on its own; mapping this to
                    // "disconnected" made scheduleReconnect() tear down the
                    // session it was about to establish.
                    "no_network" -> "connecting"
                    else -> "disconnected"
                }
                EngineHub.pushEvent("state", statusJson(mapped))
                if (mapped == "connected") {
                    reconnectStep = 0 // backoff chain only runs while offline
                    reconnectQueued.set(false)
                    WisteriaBridgeHolder.instance?.onConnectedCheckIP()
                }
                if (mapped == "connecting") reconnectQueued.set(false)
                updateNotification(mapped)
                WisteriaTileService.refresh(this@WisteriaVpnService)
                // An engine-originated "disconnected" while the user wants the
                // tunnel (wisteria runLoop device-dead, box core stop) is the
                // fast path of the watchdog: react immediately, do not idle 2s.
                // Events fired from our own teardown() are not a drop.
                if (mapped == "disconnected" && !userStopped && !isTearingDown) scheduleReconnect()
            } catch (t: Throwable) {
                android.util.Log.w("WisteriaVpn", "onState", t)
            }
        }

        override fun protect(fd: Long): Boolean = try {
            this@WisteriaVpnService.protect(fd.toInt())
        } catch (_: Throwable) {
            // An invalid/closed fd throws IllegalArgumentException; a
            // failed protect must read as "false", not crash via JNI.
            false
        }

        override fun onOpenTun(optionsJson: String?): Int {
            // establish() throws when VPN consent is revoked mid-boot (or
            // an always-on VPN conflicts) — uncaught, that exception rides
            // JNI into Go and panics the process. Any failure here is a
            // clean "-1" the box engine reports as a start error.
            try {
                val opts = try { JSONObject(optionsJson ?: "{}") } catch (_: Exception) { JSONObject() }
                // Serialize with teardown: boxTun must never be assigned after a
                // teardown already closed it (fd leak), and a Disconnect that
                // raced the box boot must not hand the dying engine a fresh tun.
                // The lock spans establish() — a few ms — and teardown only
                // holds it for the field snapshot, never while it stops the
                // engine, so the two cannot deadlock.
                synchronized(stateLock) {
                    if (isTearingDown || userStopped) return -1
                    val builder = Builder()
                        .setSession(EngineHub.profileName.ifEmpty { "Wisteria" })
                        .setMtu(opts.optInt("mtu", TUN_MTU))
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) builder.setMetered(false)
                    for (key in listOf("inet4", "inet6")) {
                        val arr = opts.optJSONArray(key) ?: continue
                        for (i in 0 until arr.length()) {
                            val cidr = arr.optString(i)
                            val slash = cidr.lastIndexOf('/')
                            if (slash <= 0) continue
                            val addr = cidr.substring(0, slash)
                            val bits = cidr.substring(slash + 1).toIntOrNull() ?: continue
                            builder.addAddress(addr, bits)
                        }
                    }
                    val routes = opts.optJSONArray("routes")
                    if (routes != null && routes.length() > 0) {
                        for (i in 0 until routes.length()) {
                            val cidr = routes.optString(i)
                            val slash = cidr.lastIndexOf('/')
                            if (slash <= 0) continue
                            val addr = cidr.substring(0, slash)
                            val bits = cidr.substring(slash + 1).toIntOrNull() ?: continue
                            builder.addRoute(addr, bits)
                        }
                    } else {
                        builder.addRoute("0.0.0.0", 0)
                    }
                    builder.addDnsServer(tunDns())
                    applySplitApps(builder)
                    val pfd = builder.establish() ?: return -1
                    // Re-check under the same lock: a Disconnect that landed
                    // during establish() must not leave the just-created fd
                    // owned by a service that is already tearing down.
                    if (isTearingDown || userStopped) {
                        runCatching { pfd.close() }
                        return -1
                    }
                    boxTun = pfd
                    return pfd.fd
                }
            } catch (t: Throwable) {
                android.util.Log.w("WisteriaVpn", "onOpenTun", t)
                EngineHub.coreLog("tun: ${t.message ?: t.javaClass.simpleName}")
                return -1
            }
        }
    }

    override fun onCreate() {
        super.onCreate()
        EngineHub.init(applicationContext)
        createChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            // Own thread: CancelConnect must not queue behind a connect()
            // that is stuck inside a blocking native engine call.
            ACTION_DISCONNECT -> Thread({
                // The QS tile delivers this via startForegroundService while
                // the app is backgrounded: answer the FGS obligation before
                // the teardown removes the notification again.
                tryStartForeground("disconnected")
                userStopped = true // cancels the watchdog and the reconnect queue
                cancelEpoch++ // invalidates CONNECT intents queued before this cancel
                stopWatchdog()
                teardown()
                stopSelf()
            }, "wisteria-disconnect").start()
            ACTION_RECONNECT -> {
                // Settings that reshape the tunnel (preset, per-app split,
                // bypass routes) only apply to a fresh Builder: tear the
                // session down and bring it back up on the same serialized
                // executor, so a Disconnect racing the restart still wins.
                val epoch = cancelEpoch
                // Answer the FGS obligation HERE, synchronously: the intent
                // usually comes through startService/startForegroundService
                // while the activity is still visible, and doing it from the
                // queued task (after a slow teardown) is what threw
                // ForegroundServiceStartNotAllowedException on Android 12+.
                tryStartForeground(currentStatusWord())
                executor.execute {
                    if (userStopped) {
                        stopSelf()
                        return@execute
                    }
                    teardown(pushState = true, keepForeground = true)
                    connect("", epoch, fgsRequired = false)
                }
            }
            else -> {
                val id = intent?.getStringExtra(EXTRA_PROFILE_ID).orEmpty()
                // The generation is read HERE, at submission time. Reading
                // cancelEpoch inside the lambda would sample it when the task
                // finally runs, i.e. after any Cancel that landed while this
                // connect sat in the queue — the check would then compare the
                // post-cancel value with itself and wave the Cancel through.
                val epoch = cancelEpoch
                // A null intent is the START_STICKY resurrection after a
                // system kill: the SYSTEM started us, so there is no
                // startForegroundService obligation — startForeground is
                // best-effort there (it throws when the app is backgrounded,
                // Android 12+). An explicit CONNECT intent came through
                // startForegroundService and MUST reach startForeground.
                val fgsRequired = intent != null
                executor.execute { connect(id, epoch, fgsRequired) }
            }
        }
        // STICKY: if the system kills the service (memory, LMK), it comes
        // back with a null intent and connect("") reconnects the active
        // profile — a VPN must not die silently. Clean stopSelf() paths
        // stay unrestarted (STICKY does not resurrect after a clean stop).
        return START_STICKY
    }

    override fun onRevoke() {
        // The system (or the user in settings) revoked VPN consent.
        userStopped = true // without consent a reconnect loop is pointless
        cancelEpoch++ // a queued connect must not fight the revoked consent
        stopWatchdog()
        executor.execute {
            teardown()
            stopSelf()
        }
    }

    override fun onDestroy() {
        userStopped = true
        stopWatchdog()
        // The debounced network-switch task must not survive the service.
        synchronized(netSwitchLock) {
            netSwitchFuture?.cancel(false)
            netSwitchFuture = null
        }
        watchdogExec.shutdownNow()
        // Tear down synchronously: queueing it behind executor.shutdown()
        // used to drop the task (shutdown() rejects new work), leaking the
        // engine process and the tun fd on every service destruction.
        teardown(pushState = false)
        executor.shutdownNow()
        super.onDestroy()
    }

    private fun connect(profileIdParam: String, myCancelEpoch: Long, fgsRequired: Boolean = true) {
        val store = AndroidStore(this)
        // Resolve the target BEFORE the duplicate check: "Connect" is also
        // the switch-while-connected call (the frontend does SetActive +
        // Connect on a profile tap), so a live session must be told apart
        // from a real duplicate by *which* profile it runs. An empty request
        // while a session is up means "the one that is running" — never tear
        // a live tunnel down just because the active pointer is empty.
        val targetId = profileIdParam.ifEmpty {
            store.activeId().ifEmpty { EngineHub.profileId }
        }

        var switchFrom: String? = null
        var myEpoch: Long = synchronized(stateLock) {
            when {
                // `starting` closes the gap before the engine flags are set
                // (startBox sets engineIsBox only after the config build,
                // startWisteria — tun only after establish()): two rapid
                // CONNECTs could otherwise start the engine twice.
                starting -> -1L
                tun != null || engineIsBox -> {
                    if (targetId.isNotEmpty() && targetId == EngineHub.profileId) {
                        -1L // same profile is already up: duplicate CONNECT
                    } else {
                        // Different profile: momentary switch. Claim the start
                        // slot now so a second tap cannot slip in, then tear
                        // the old session down outside stateLock.
                        switchFrom = EngineHub.profileId
                        starting = true
                        teardownEpoch
                    }
                }
                else -> {
                    starting = true
                    teardownEpoch
                }
            }
        }
        if (myEpoch < 0L) {
            // Duplicate CONNECT (double-tap, STICKY redelivery): the start
            // itself is a no-op, but every startForegroundService() call
            // must still be answered with startForeground() within the
            // system timeout, or the app is crashed ~10 s later. Re-post
            // the notification with the engine's current state word.
            if (!tryStartForeground(currentStatusWord()) && fgsRequired) {
                stopSelf()
            }
            return
        }
        // startForeground before anything slow: a startForegroundService
        // (bridge, BootReceiver) must reach startForeground() even when the
        // profile turns out to be missing and the service stops itself.
        // The try/finally spans everything from here on: `starting` must be
        // cleared on ANY exit path, or the next Connect would forever land
        // in the duplicate-start branch above.
        if (!tryStartForeground() && fgsRequired) {
            // startForeground threw (ForegroundServiceStartNotAllowedException:
            // the app slipped into the background between the start call and
            // this point, Android 12+). Leaving the obligation unanswered
            // crashes the app ~10 s later — stop cleanly and tell the UI.
            synchronized(stateLock) { starting = false }
            EngineHub.appLog("подключение: приложение в фоне — откройте его и повторите")
            abortSession(null, "приложение в фоне — откройте его для подключения")
            return
        }
        try {
            if (switchFrom != null) {
                // A Disconnect may have landed while we were claiming the
                // start slot — it wins over a switch that only just began.
                if (cancelEpoch != myCancelEpoch) return
                EngineHub.appLog("переключение профиля — перезапуск сессии")
                // keepForeground: the switch is an internal restart, the
                // following startForeground() must not be a background FGS
                // start (Android 12+).
                teardown(pushState = true, keepForeground = true)
                // teardown() bumps teardownEpoch; the engine start below must
                // compare against the post-teardown generation, or its own
                // teardown reads as "a Disconnect raced me".
                myEpoch = synchronized(stateLock) { teardownEpoch }
            }

            val id = targetId
            val profile = store.profile(id)
                ?: run {
                    EngineHub.appLog("подключение: профиль не найден")
                    abortSession("", "профиль не найден")
                    return
                }

            // A Disconnect may have landed while this connect sat queued on
            // the executor: its cancelEpoch++ differs from the generation
            // captured at onStartCommand, so the queued connect must not
            // bring the tunnel up after the user pressed Cancel. (Cancels
            // landing DURING the engine start are covered by the
            // epochChanged() checks in startWisteria / startBox.)
            if (cancelEpoch != myCancelEpoch) return

            // Start under engineLock: a teardown on the Disconnect thread
            // may still be stopping the previous engine even though its
            // fields are already cleared (and its Cancel may have landed
            // after the check above — re-check under the lock).
            synchronized(engineLock) {
                if (cancelEpoch != myCancelEpoch) return

                // The user wants a tunnel now: arm the watchdog / reconnect path.
                userStopped = false
                sessionStartEpoch = System.currentTimeMillis() / 1000
                reconnectStep = 0
                reconnectQueued.set(false)

                if (profile.optString("type") == "wisteria") {
                    startWisteria(profile, myEpoch)
                } else {
                    startBox(profile, myEpoch)
                }
            }
        } catch (e: Throwable) {
            // establish()/store failures must not wedge `starting` (see the
            // finally below) nor vanish silently: surface the real cause and
            // — critically — take the "Connecting…" notification down and
            // stop the service, so the watchdog has nothing to loop on.
            android.util.Log.d("WisteriaVpn", "connect failed", e)
            EngineHub.appLog("подключение не удалось: ${trimErr(e.message)}")
            abortSession("", trimErr(e.message) ?: e.javaClass.simpleName)
        } finally {
            synchronized(stateLock) { starting = false }
        }
    }

    /**
     * Terminal failure of a start attempt: no session is desired any more,
     * so stop the watchdog, drop the foreground notification (otherwise a
     * stuck "Connecting…" outlives the attempt) and stop the service. Every
     * early exit from connect()/startWisteria()/startBox() funnels through
     * here — a return that skipped even one of these left the notification
     * up and the reconnect watchdog armed.
     */
    private fun abortSession(type: String?, message: String) {
        userStopped = true
        sessionStartEpoch = 0
        stopWatchdog()
        EngineHub.pushEvent("state", statusJson("disconnected", message, type))
        WisteriaTileService.refresh(this)
        try {
            ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        } catch (_: Throwable) {
        }
        stopSelf()
    }

    /** True when a teardown happened since the caller captured its epoch:
     *  the connect must honor a Disconnect that raced the engine start. */
    private fun epochChanged(myEpoch: Long): Boolean =
        synchronized(stateLock) { teardownEpoch != myEpoch }

    private fun presetOf(profile: JSONObject, settings: JSONObject): String {
        val per = profile.optJSONObject("fields")?.optString("preset").orEmpty()
        val v = per.ifEmpty { settings.optString("wisteria_preset", "stealth") }
        return when (v) { "fast", "balance", "stealth" -> v; else -> "stealth" }
    }

    private fun startWisteria(profile: JSONObject, myEpoch: Long) {
        // "Do not proxy Russian sites" — the Android counterpart of the
        // desktop routing.mode=bypass_ru. VpnService cannot match domains,
        // so the RU CIDR list is subtracted from the 0.0.0.0/0 route and the
        // OS sends those destinations out the physical interface.
        // A prefix the framework refuses must not kill the tunnel: losing
        // one RU site to the tunnel is cheaper than losing the VPN.
        val settings = AndroidStore(this).settings()
        val bypassRU = !settings.optBoolean("no_bypass_ru", false)
        val customCidrs = settings.optString("wisteria_direct_cidrs")
        val canExclude = Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU

        // The exclusion list is built once; establishGuarded() may need a
        // second Builder without it (see there).
        val exclusions: List<android.net.IpPrefix> = if (canExclude) {
            val out = ArrayList<android.net.IpPrefix>(9000)
            if (bypassRU) out.addAll(RuRoutes.ipv4(this))
            out.addAll(RuRoutes.parseList(customCidrs))
            out
        } else {
            emptyList()
        }
        if (!canExclude && (bypassRU || customCidrs.isNotBlank())) {
            // Say it out loud instead of silently ignoring the checkbox:
            // excludeRoute appeared in Android 13, and a user who ticks the
            // box on Android 12 must not wonder why their bank still sees
            // a foreign IP.
            EngineHub.appLog(
                "маршрутизация: обход по подсетям требует Android 13+ " +
                    "(на этом устройстве весь трафик идёт в туннель)"
            )
        }

        fun buildBuilder(useExclusions: Boolean): Builder {
            val b = Builder()
                .addAddress(TUN_ADDRESS, TUN_PREFIX)
                .addRoute("0.0.0.0", 0)
                // The wisteria engine relays IPv4 only. Without a v6 route the
                // OS sends IPv6 straight out the physical interface — the
                // tunnel would leak every dual-stack connection. Route ::/0
                // into the TUN (no v6 address) so v6 dies inside it instead.
                .addRoute("::", 0)
                .addDnsServer(tunDns())
                .setMtu(TUN_MTU)
                .setSession(profile.optString("name").ifEmpty { "Wisteria" })
            applySplitApps(b)
            if (useExclusions) {
                for (prefix in exclusions) {
                    try {
                        b.excludeRoute(prefix)
                    } catch (e: Exception) {
                        android.util.Log.w("WisteriaVpn", "excludeRoute $prefix", e)
                    }
                }
            }
            return b
        }

        val started = System.currentTimeMillis()
        val fd = establishGuarded(::buildBuilder)
        if (fd == null) {
            EngineHub.appLog("VpnService.establish() вернул null — согласие на VPN отозвано?")
            abortSession("wisteria", "VpnService: согласие на VPN не выдано")
            return
        }
        if (exclusions.isNotEmpty()) {
            EngineHub.appLog(
                "маршрутизация: ${exclusions.size} подсетей идут напрямую " +
                    "(${System.currentTimeMillis() - started} мс)"
            )
        }
        tun = fd
        EngineHub.resetDiag() // chips must describe this session, not the previous one
        EngineHub.profileId = profile.optString("id")
        EngineHub.profileName = profile.optString("name")
        EngineHub.profileType = "wisteria"
        EngineHub.appLog("подключение к «${EngineHub.profileName}»…")

        // The engine takes the whole server list from the profile (bundle
        // links carry mirrors) and rotates targets on every attempt. Manual
        // profiles have no stored list — TlsTransport builds the single dial
        // row from host:port (+ pinned pub, + TLS transport fields, wave 4);
        // a config with neither a server list nor an endpoint is rejected.
        val cfg = JSONObject()
            .put("priv", profile.optString("client_priv"))
            .put("name", profile.optString("name"))
            .put("preset", presetOf(profile, settings))
            .put("servers", TlsTransport.engineServers(profile))
            .toString()

        EngineHub.pushEvent("state", statusJson("connecting"))
        try {
            Mobile.start(cfg, fd.fd.toLong(), callback)
            // The native engine has no netlink visibility on Android: watch
            // the default network ourselves so a Wi-Fi ⇄ LTE switch rebuilds
            // the session instead of black-holing it.
            registerWisteriaNetworkMonitor()
            // A Disconnect may have landed while we were establishing or
            // starting; honor it by tearing the just-started engine down.
            if (epochChanged(myEpoch)) {
                EngineHub.appLog("отмена пришла во время запуска — останавливаю")
                teardown(pushState = true)
                stopSelf()
            } else {
                startWatchdog()
            }
        } catch (e: Exception) {
            EngineHub.coreLog("start failed: ${e.message}")
            // Show the failure instead of silently swallowing it: the VPN
            // screen renders status.error in the red box (v0.4.2's
            // "connecting forever" looked exactly like a swallowed error).
            // teardown() first: the tun is already assigned here.
            teardown(pushState = false)
            abortSession("wisteria", trimErr(e.message) ?: e.javaClass.simpleName)
        }
    }

    /** True for the "IPC payload too big" family thrown by establish() when
     *  the Builder carries thousands of excluded routes: TransactionTooLarge
     *  on the parcel itself, or the framework's own route-count refusal. */
    private fun isBinderOverflow(t: Throwable): Boolean {
        var cur: Throwable? = t
        while (cur != null) {
            if (cur is android.os.TransactionTooLargeException) return true
            val m = (cur.message ?: "").lowercase()
            if (m.contains("transactiontoolarge") || m.contains("too many routes") ||
                m.contains("too many addresses") || m.contains("too large")
            ) return true
            cur = cur.cause
        }
        return false
    }

    /**
     * establish() with a degradation path. Excluding ~8.7k RU CIDRs writes a
     * large parcel to the system VPN service; on some devices/ROMs that
     * overflow the binder transaction and the whole VPN fails to come up.
     * A failed exclusion list is a routing nicety, not the tunnel: retry
     * once with a plain Builder and say what was lost in the log.
     */
    private fun establishGuarded(build: (Boolean) -> Builder): ParcelFileDescriptor? = try {
        build(true).establish()
    } catch (t: Throwable) {
        if (!isBinderOverflow(t)) throw t
        EngineHub.appLog(
            "маршрутизация: список исключений слишком велик для системы " +
                "(${t.javaClass.simpleName}) — поднимаю туннель без обхода подсетей"
        )
        android.util.Log.w("WisteriaVpn", "establish with exclusions failed, retrying plain", t)
        build(false).establish()
    }

    /** sing-box path: vless / vmess / trojan / ss / hysteria2 / tuic. */
    private fun startBox(profile: JSONObject, myEpoch: Long) {
        val raw = profile.optString("raw")
        val name = profile.optString("name").ifEmpty { "Wisteria" }
        val config = try {
            SingboxLinks.buildConfig(
                raw,
                name,
                AndroidStore(this).settings(),
                cachePath = "${filesDir.absolutePath}/cache.db",
                rulesetPaths = Rulesets.paths(this),
            )
        } catch (e: Exception) {
            EngineHub.appLog("профиль повреждён: ${e.message}")
            abortSession(profile.optString("type"), "профиль повреждён: ${trimErr(e.message)}")
            return
        }
        engineIsBox = true
        EngineHub.resetDiag() // chips must describe this session, not the previous one
        EngineHub.profileId = profile.optString("id")
        EngineHub.profileName = name
        EngineHub.profileType = profile.optString("type")
        EngineHub.appLog("подключение к «$name» (${EngineHub.profileType}, sing-box ${boxVersion()})…")
        EngineHub.pushEvent("state", statusJson("connecting"))
        try {
            Mobile.boxStart(config, callback)
            registerBoxNetworkMonitor()
            if (epochChanged(myEpoch)) {
                EngineHub.appLog("отмена пришла во время запуска — останавливаю")
                teardown(pushState = true)
                stopSelf()
            } else {
                startWatchdog()
            }
        } catch (e: Exception) {
            EngineHub.coreLog("sing-box: ${e.message}")
            // See startWisteria: surface the failure on the VPN screen.
            teardown(pushState = false)
            abortSession(EngineHub.profileType, trimErr(e.message) ?: e.javaClass.simpleName)
        }
    }

    private fun boxVersion(): String =
        try { Mobile.boxVersion() } catch (_: Throwable) { "-" }

    /** Engine errors go into the VPN screen's red box; keep them one line. */
    private fun trimErr(msg: String?): String? {
        val m = msg?.trim().orEmpty()
        if (m.isEmpty()) return null
        return if (m.length <= 160) m else m.take(157) + "…"
    }

    /**
     * The physical network carrying traffic. ConnectivityManager's
     * activeNetwork is the VPN itself once we are up, and
     * registerDefaultNetworkCallback() then replays *our own TUN* as the
     * "default" — the box core was told to bind its outbound sockets to the
     * tun interface and the wisteria monitor read its own interface as a
     * network switch. Both are non-VPN-only now.
     */
    @Suppress("DEPRECATION") // allNetworks: still the only way to enumerate physical networks
    private fun physicalNetwork(cm: ConnectivityManager): Network? {
        var fallback: Network? = null
        try {
            for (n in cm.allNetworks) {
                val caps = cm.getNetworkCapabilities(n) ?: continue
                if (!caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)) continue
                if (caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN)) continue
                if (caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED)) return n
                if (fallback == null) fallback = n
            }
        } catch (_: Throwable) {
        }
        return fallback
    }

    /** A callback registration that only ever sees physical networks. */
    private fun nonVpnNetworkRequest(): NetworkRequest =
        NetworkRequest.Builder()
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)
            .build()

    /**
     * Android blocks netlink interface dumps, so the box's default
     * interface monitor is fed from ConnectivityManager: initial state
     * right after the start, then on every network change.
     */
    private fun registerBoxNetworkMonitor() {
        val cm = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager

        fun reportDefault() {
            val active = physicalNetwork(cm) ?: run {
                Mobile.boxUpdateDefaultInterface("", -1, false, false)
                return
            }
            val name = cm.getLinkProperties(active)?.interfaceName
            if (name.isNullOrEmpty()) {
                Mobile.boxUpdateDefaultInterface("", -1, false, false)
                return
            }
            val idx = try {
                java.net.NetworkInterface.getByName(name)?.index ?: -1
            } catch (_: Exception) { -1 }
            val caps = cm.getNetworkCapabilities(active)
            val expensive =
                caps?.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED) == false
            Mobile.boxUpdateDefaultInterface(name, idx, expensive, false)
        }

        val cb = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                reportDefault()
            }

            // The default network died (Wi-Fi dropped before the cellular
            // one came up): the box core must forget the interface it was
            // told about, or it keeps sending on a dead one.
            override fun onLost(network: Network) {
                reportDefault()
            }

            override fun onLinkPropertiesChanged(network: Network, properties: android.net.LinkProperties) {
                reportDefault()
            }
        }
        boxNetCallback = cb
        try {
            // NOT registerDefaultNetworkCallback(): it follows the VPN once
            // the tun is up and reported "tun0" as the physical interface.
            cm.registerNetworkCallback(nonVpnNetworkRequest(), cb)
            // The callback may deliver onAvailable on a binder thread later;
            // seed the core with the current network right away so the first
            // dial does not race the callback.
            reportDefault()
        } catch (e: Exception) {
            EngineHub.coreLog("network monitor: ${e.message}")
        }
    }

    private fun unregisterBoxNetworkMonitor() {
        boxNetCallback?.let {
            try {
                val cm = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
                cm.unregisterNetworkCallback(it)
            } catch (_: Throwable) {
            }
        }
        boxNetCallback = null
    }

    /**
     * Default-network watchdog for the native wisteria engine. On a real
     * Wi-Fi ⇄ cellular switch the engine's sockets are bound to the old
     * network; rebuilding the session is cheaper and more reliable than
     * waiting for its health monitor to declare the tunnel dead. Debounced
     * by 600 ms because the switch itself fires onLost + onAvailable for
     * several networks in a row.
     */
    private fun registerWisteriaNetworkMonitor() {
        unregisterWisteriaNetworkMonitor()
        val cm = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
        // Registration replays onAvailable for the already-present networks;
        // remember the physical one so the replay is not read as a switch.
        lastActiveNetHandle = try {
            physicalNetwork(cm)?.networkHandle
        } catch (_: Throwable) {
            null
        }
        val cb = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) = onMaybeSwitched()

            // onLost often lands BEFORE the new default network is picked:
            // with no active network there is nothing to compare yet, and the
            // following onAvailable handles the switch. Never tear down on a
            // null handle here.
            override fun onLost(network: Network) = onMaybeSwitched()

            private fun onMaybeSwitched() {
                // Only physical networks reach this callback (the request
                // carries NOT_VPN), so bringing our own tun up/down cannot
                // be mistaken for a Wi-Fi ⇄ LTE switch.
                val currentHandle = try {
                    physicalNetwork(cm)?.networkHandle
                } catch (_: Throwable) {
                    null
                } ?: return
                if (currentHandle == lastActiveNetHandle) return
                lastActiveNetHandle = currentHandle
                if (userStopped || isTearingDown) return
                synchronized(netSwitchLock) {
                    netSwitchFuture?.cancel(false)
                    netSwitchFuture = try {
                        watchdogExec.schedule({
                            executor.execute {
                                if (userStopped || isTearingDown) return@execute
                                val epoch = cancelEpoch
                                EngineHub.appLog("сеть изменилась — переподключение")
                                reconnectStep = 0
                                reconnectQueued.set(false)
                                teardown(pushState = true, keepForeground = true)
                                connect("", epoch, fgsRequired = false)
                            }
                        }, 600L, TimeUnit.MILLISECONDS)
                    } catch (_: Throwable) {
                        null // executor already shut down (service dying)
                    }
                }
            }
        }
        wisteriaNetCallback = cb
        try {
            // Physical networks only — see nonVpnNetworkRequest(). The old
            // registerDefaultNetworkCallback() followed the tun itself and
            // fired "сеть изменилась" right after every connect.
            cm.registerNetworkCallback(nonVpnNetworkRequest(), cb)
        } catch (e: Exception) {
            EngineHub.coreLog("network monitor: ${e.message}")
        }
    }

    private fun unregisterWisteriaNetworkMonitor() {
        wisteriaNetCallback?.let {
            try {
                val cm = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
                cm.unregisterNetworkCallback(it)
            } catch (_: Throwable) {
            }
        }
        wisteriaNetCallback = null
        lastActiveNetHandle = null
        synchronized(netSwitchLock) {
            netSwitchFuture?.cancel(false)
            netSwitchFuture = null
        }
    }

    private fun teardown(pushState: Boolean = true, keepForeground: Boolean = false) {
        stopWatchdog()
        // Depth counter, not a boolean: overlapping passes (a user
        // Disconnect + onDestroy, or an internal reconnect) each add one and
        // remove one; only the outermost exit clears the flag, so a shorter
        // pass can never re-arm the "engine died on its own" path under a
        // longer one.
        tearingDownDepth.incrementAndGet()
        try {
            // Everything happens under engineLock, including the metadata /
            // monitor / notification cleanup. This is what makes a finishing
            // old Disconnect thread harmless: a Connect that starts a new
            // session holds the same lock for its whole start, so the old
            // teardown physically cannot wipe the fresh profileId,
            // network monitor or foreground notification of the new session.
            //
            // The engine stop is inside the lock for the same reason, but the
            // field snapshot uses its own short stateLock critical section:
            // Mobile.stop()/boxStop() can block while the engine is inside
            // callback.onOpenTun(), which itself takes stateLock — holding it
            // across the stop would deadlock.
            synchronized(engineLock) {
                val (wasBox, boxFd, tunFd) = synchronized(stateLock) {
                    teardownEpoch++
                    val snap = Triple(engineIsBox, boxTun, tun)
                    boxTun = null
                    engineIsBox = false
                    tun = null
                    snap
                }
                if (wasBox) {
                    unregisterBoxNetworkMonitor()
                    try {
                        Mobile.boxStop()
                    } catch (_: Throwable) {
                    }
                    try {
                        boxFd?.close()
                    } catch (_: Throwable) {
                    }
                } else {
                    try {
                        Mobile.stop()
                    } catch (_: Throwable) {
                    }
                }
                unregisterWisteriaNetworkMonitor()
                try {
                    tunFd?.close()
                } catch (_: Throwable) {
                }
                EngineHub.profileId = ""
                EngineHub.profileName = ""
                EngineHub.profileType = ""
                sessionStartEpoch = 0 // the pushed "disconnected" must not carry a stale started_at
                if (pushState) EngineHub.pushEvent("state", statusJson("disconnected"))
                WisteriaTileService.refresh(this)
                // Internal restarts (reconnect, network switch, profile
                // switch) keep the foreground notification: dropping it here
                // made the following startForeground() a background FGS start
                // on Android 12+.
                if (!keepForeground) {
                    try {
                        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
                    } catch (_: Throwable) {
                    }
                }
            }
        } finally {
            // Only after the whole pass: the engine Stop() above fires
            // onState("disconnected") synchronously — that is our teardown,
            // not a drop, and must not queue a reconnect.
            tearingDownDepth.decrementAndGet()
        }
    }

    /** The desktop Status shape the frontend renders. */
    private fun statusJson(state: String, error: String? = null, type: String? = null): JSONObject =
        WisteriaBridge.staticStatusJson(
            state,
            type ?: if (state == "disconnected") "" else EngineHub.profileType,
            error,
            sessionStartEpoch,
        )

    /** The engine's current state mapped onto the notification words; used
     *  when a duplicate CONNECT start must answer startForeground() without
     *  clobbering the live "Secured" notification with "Подключение…". */
    private fun currentStatusWord(): String = try {
        when (JSONObject(Mobile.status()).optString("state")) {
            "connected" -> "connected"
            else -> "connecting"
        }
    } catch (_: Throwable) {
        "connecting"
    }

    /** Per-app split tunnel: "bypass" keeps the listed apps on the physical
     *  interface (banks / RU services see the real IP without CIDR hacks),
     *  "only" sends just the listed apps through the tunnel. A package the
     *  framework refuses (uninstalled meanwhile) must not kill the tunnel. */
    private fun applySplitApps(builder: Builder) {
        val s = AndroidStore(this).settings()
        val mode = s.optString("split_mode", "off")
        if (mode != "bypass" && mode != "only") return
        val arr = s.optJSONArray("split_apps") ?: JSONArray()
        if (mode == "only" && arr.length() == 0) {
            EngineHub.appLog("split-tunnel: «только выбранные» с пустым списком — весь трафик идёт в туннель")
            return
        }
        var added = 0
        for (i in 0 until arr.length()) {
            val pkg = arr.optString(i)
            if (pkg.isEmpty() || pkg == packageName) continue
            try {
                if (mode == "only") builder.addAllowedApplication(pkg)
                else builder.addDisallowedApplication(pkg)
                added++
            } catch (e: Exception) {
                android.util.Log.w("WisteriaVpn", "split $pkg", e)
            }
        }
        if (added > 0) {
            EngineHub.appLog(
                "split-tunnel: ${if (mode == "only") "в туннель только" else "мимо туннеля"} $added прил."
            )
        }
    }

    /** TUN DNS server: the user's remote_dns when it is a plain IP (the
     *  desktop routing setting both engines understand), else the 8.8.8.8
     *  default. DoT/DoH forms (tls://, https://) cannot ride
     *  addDnsServer — the box engine applies them in its own config (and
     *  hijacks DNS anyway), so a non-IP value falls back to the default
     *  instead of wedging the tunnel. */
    private fun tunDns(): String {
        val raw = AndroidStore(this).settings().optString("remote_dns", "").trim()
        if (raw.isEmpty()) return DNS
        val candidate = raw.substringAfter("://").substringBefore('/').trim()
            .ifEmpty { raw }
        if (candidate.isNotEmpty() &&
            android.util.Patterns.IP_ADDRESS.matcher(candidate).matches()
        ) return candidate
        return DNS
    }

    private fun createChannel() {
        val nm = getSystemService(NotificationManager::class.java) ?: return
        nm.createNotificationChannel(
            NotificationChannel(CHANNEL_ID, "Wisteria VPN", NotificationManager.IMPORTANCE_LOW)
        )
    }

    private fun startForegroundWith(state: String = "connecting") {
        ServiceCompat.startForeground(
            this, NOTIF_ID, buildNotification(state),
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE
            else
                0
        )
    }

    /** startForeground that cannot crash the executor thread: Android 12+
     *  throws ForegroundServiceStartNotAllowedException when the app is
     *  backgrounded (STICKY resurrection, a connect queued behind a slow
     *  teardown while the user pressed Home). Returns false instead; the
     *  caller decides whether the FGS obligation forces a clean stop. */
    private fun tryStartForeground(state: String = "connecting"): Boolean = try {
        startForegroundWith(state)
        true
    } catch (t: Throwable) {
        android.util.Log.w("WisteriaVpn", "startForeground", t)
        false
    }

    private fun updateNotification(state: String) {
        val nm = getSystemService(NotificationManager::class.java) ?: return
        nm.notify(NOTIF_ID, buildNotification(state))
    }

    private fun buildNotification(state: String): Notification {
        val contentIntent = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        // The notification must carry its own Disconnect: opening the app to
        // stop the tunnel was the only way before. On API 26+ this service is
        // already a foreground service running from onStartCommand's
        // tryStartForeground(), and a plain getService() PendingIntent on an
        // FGS-targeted intent is refused by the background-start rules —
        // getForegroundService() matches how onStartCommand answers the
        // obligation, so the action keeps working from the shade.
        val disconnectIntent = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            PendingIntent.getForegroundService(
                this, 1,
                Intent(this, WisteriaVpnService::class.java).setAction(ACTION_DISCONNECT),
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
            )
        } else {
            PendingIntent.getService(
                this, 1,
                Intent(this, WisteriaVpnService::class.java).setAction(ACTION_DISCONNECT),
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
            )
        }
        val word = when (state) {
            "connected" -> getString(R.string.status_connected)
            "connecting" -> getString(R.string.status_connecting)
            else -> getString(R.string.status_idle)
        }
        // Name the profile while a session is up: two Wisteria accounts in
        // the shade were indistinguishable from the outside.
        val name = EngineHub.profileName.trim()
        val text = if (name.isNotEmpty() && state != "disconnected") "$word · $name" else word
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_notif)
            .setContentTitle("Wisteria")
            .setContentText(text)
            .setContentIntent(contentIntent)
            .addAction(0, getString(R.string.notif_disconnect), disconnectIntent)
            .setOngoing(true)
            .build()
    }
}

/**
 * The service and the bridge live in separate components; the bridge
 * instance that owns check_ext_ip after connect is registered here by the
 * activity so the service can reach it without leaking the webview.
 */
object WisteriaBridgeHolder {
    @Volatile
    var instance: WisteriaBridge? = null
}

/**
 * The profile waiting for the system VPN consent dialog, process-wide.
 * It used to live on the activity/bridge instances: a rotation (or the
 * restartApp relaunch) while the dialog was up recreated both, the grant
 * then matched a null pending id and was silently dropped — the user
 * tapped "Connect", allowed the VPN, and nothing happened.
 */
object VpnConsentHolder {
    @Volatile
    var pendingId: String? = null
}
