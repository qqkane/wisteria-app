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
    private val executor = Executors.newSingleThreadExecutor()
    private var engineIsBox = false

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

    /** True while teardown() runs: its engine stop must not be read as an
     *  engine-originated death. */
    @Volatile
    private var tearingDown = false

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
                connect("", epoch)
            }
        }, delay.toLong(), TimeUnit.SECONDS)
    }

    private val callback = object : Callback {
        override fun onLog(line: String?) {
            EngineHub.coreLog(line ?: "")
        }

        override fun onState(state: String?) {
            val mapped = when (state) {
                "connecting", "retrying" -> "connecting"
                "connected" -> "connected"
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
            // An engine-originated "disconnected" while the user wants the
            // tunnel (wisteria runLoop device-dead, box core stop) is the
            // fast path of the watchdog: react immediately, do not idle 2s.
            // Events fired from our own teardown() are not a drop.
            if (mapped == "disconnected" && !userStopped && !tearingDown) scheduleReconnect()
        }

        override fun protect(fd: Long): Boolean =
            this@WisteriaVpnService.protect(fd.toInt())

        override fun onOpenTun(optionsJson: String?): Int {
            val opts = try { JSONObject(optionsJson ?: "{}") } catch (_: Exception) { JSONObject() }
            // Serialize with teardown: boxTun must never be assigned after a
            // teardown already closed it (fd leak), and a Disconnect that
            // raced the box boot must not hand the dying engine a fresh tun.
            // The lock spans establish() — a few ms — and teardown (the
            // other stateLock user) simply waits it out.
            synchronized(stateLock) {
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
                val pfd = builder.establish() ?: return -1
                boxTun = pfd
                return pfd.fd
            }
        }
    }

    override fun onCreate() {
        super.onCreate()
        createChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            // Own thread: CancelConnect must not queue behind a connect()
            // that is stuck inside a blocking native engine call.
            ACTION_DISCONNECT -> Thread({
                userStopped = true // cancels the watchdog and the reconnect queue
                cancelEpoch++ // invalidates CONNECT intents queued before this cancel
                stopWatchdog()
                teardown()
                stopSelf()
            }, "wisteria-disconnect").start()
            else -> {
                val id = intent?.getStringExtra(EXTRA_PROFILE_ID).orEmpty()
                // The generation is read HERE, at submission time. Reading
                // cancelEpoch inside the lambda would sample it when the task
                // finally runs, i.e. after any Cancel that landed while this
                // connect sat in the queue — the check would then compare the
                // post-cancel value with itself and wave the Cancel through.
                val epoch = cancelEpoch
                executor.execute { connect(id, epoch) }
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
        watchdogExec.shutdownNow()
        executor.execute {
            teardown()
        }
        executor.shutdown()
        super.onDestroy()
    }

    private fun connect(profileIdParam: String, myCancelEpoch: Long) {
        val myEpoch = synchronized(stateLock) {
            // `starting` closes the gap before the engine flags are set
            // (startBox sets engineIsBox only after the config build,
            // startWisteria — tun only after establish()): two rapid
            // CONNECTs could otherwise start the engine twice.
            if (tun != null || engineIsBox || starting) null else {
                starting = true
                teardownEpoch
            }
        }
        if (myEpoch == null) {
            // Duplicate CONNECT (double-tap, STICKY redelivery): the start
            // itself is a no-op, but every startForegroundService() call
            // must still be answered with startForeground() within the
            // system timeout, or the app is crashed ~10 s later. Re-post
            // the notification with the engine's current state word.
            startForegroundWith(currentStatusWord())
            return
        }
        // startForeground before anything slow: a startForegroundService
        // (bridge, BootReceiver) must reach startForeground() even when the
        // profile turns out to be missing and the service stops itself.
        // The try/finally spans everything from here on: `starting` must be
        // cleared on ANY exit path, or the next Connect would forever land
        // in the duplicate-start branch above.
        startForegroundWith()
        try {
            val store = AndroidStore(this)
            val id = profileIdParam.ifEmpty { store.activeId() }
            val profile = store.profile(id)
                ?: run {
                    EngineHub.appLog("подключение: профиль не найден")
                    EngineHub.pushEvent("state", statusJson("disconnected", "профиль не найден"))
                    userStopped = true
                    stopSelf()
                    return
                }

            // A Disconnect may have landed while this connect sat queued on
            // the executor: its cancelEpoch++ differs from the generation
            // captured at onStartCommand, so the queued connect must not
            // bring the tunnel up after the user pressed Cancel. (Cancels
            // landing DURING the engine start are covered by the
            // epochChanged() checks in startWisteria / startBox.)
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
        } catch (e: Throwable) {
            // establish()/store failures must not wedge `starting` (see the
            // finally above) nor vanish silently: surface the real cause.
            android.util.Log.d("WisteriaVpn", "connect failed", e)
            EngineHub.appLog("подключение не удалось: ${trimErr(e.message)}")
            EngineHub.pushEvent("state", statusJson("disconnected", trimErr(e.message)))
        } finally {
            synchronized(stateLock) { starting = false }
        }
    }

    /** True when a teardown happened since the caller captured its epoch:
     *  the connect must honor a Disconnect that raced the engine start. */
    private fun epochChanged(myEpoch: Long): Boolean =
        synchronized(stateLock) { teardownEpoch != myEpoch }

    private fun startWisteria(profile: JSONObject, myEpoch: Long) {
        val builder = Builder()
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
        val fd = builder.establish()
        if (fd == null) {
            EngineHub.appLog("VpnService.establish() вернул null — согласие на VPN отозвано?")
            EngineHub.pushEvent("state", statusJson("disconnected", "VpnService: согласие на VPN не выдано"))
            stopSelf()
            return
        }
        tun = fd
        EngineHub.resetDiag() // chips must describe this session, not the previous one
        EngineHub.profileId = profile.optString("id")
        EngineHub.profileName = profile.optString("name")
        EngineHub.profileType = "wisteria"
        EngineHub.appLog("подключение к «${EngineHub.profileName}»…")

        // The engine takes the whole server list from the profile (bundle
        // links carry mirrors) and rotates targets on every attempt.
        val servers = profile.optJSONArray("servers") ?: JSONArray()
        val cfg = JSONObject()
            .put("priv", profile.optString("client_priv"))
            .put("name", profile.optString("name"))
            .put("servers", servers)
            .toString()

        EngineHub.pushEvent("state", statusJson("connecting"))
        try {
            Mobile.start(cfg, fd.fd.toLong(), callback)
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
            teardown(pushState = false)
            userStopped = true // a failed manual connect is not a dropped session
            stopSelf()
            EngineHub.pushEvent("state", statusJson("disconnected", trimErr(e.message)))
        }
    }

    /** sing-box path: vless / vmess / trojan / ss / hysteria2 / tuic. */
    private fun startBox(profile: JSONObject, myEpoch: Long) {
        val raw = profile.optString("raw")
        val name = profile.optString("name").ifEmpty { "Wisteria" }
        val config = try {
            SingboxLinks.buildConfig(raw, name, AndroidStore(this).settings(), cachePath = "${filesDir.absolutePath}/cache.db")
        } catch (e: Exception) {
            EngineHub.appLog("профиль повреждён: ${e.message}")
            EngineHub.pushEvent("state", statusJson("disconnected", "профиль повреждён: ${trimErr(e.message)}"))
            stopSelf()
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
            userStopped = true // a failed manual connect is not a dropped session
            stopSelf()
            EngineHub.pushEvent("state", statusJson("disconnected", trimErr(e.message)))
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
     * Android blocks netlink interface dumps, so the box's default
     * interface monitor is fed from ConnectivityManager: initial state
     * right after the start, then on every network change.
     */
    private fun registerBoxNetworkMonitor() {
        val cm = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
        val cb = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                reportDefault()
            }

            override fun onLinkPropertiesChanged(network: Network, properties: android.net.LinkProperties) {
                reportDefault()
            }

            private fun reportDefault() {
                val active = cm.activeNetwork
                val lp = active?.let { cm.getLinkProperties(it) }
                val name = lp?.interfaceName
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
        }
        boxNetCallback = cb
        try {
            cm.registerDefaultNetworkCallback(cb)
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

    private fun teardown(pushState: Boolean = true) {
        stopWatchdog()
        tearingDown = true
        try {
            synchronized(stateLock) {
            teardownEpoch++
            if (engineIsBox) {
                unregisterBoxNetworkMonitor()
                try {
                    Mobile.boxStop()
                } catch (_: Throwable) {
                }
                boxTun?.let {
                    try {
                        it.close()
                    } catch (_: Throwable) {
                    }
                }
                boxTun = null
                engineIsBox = false
            } else {
                try {
                    Mobile.stop()
                } catch (_: Throwable) {
                }
            }
            tun?.let {
                try {
                    it.close()
                } catch (_: Throwable) {
                }
            }
            tun = null
        }
        EngineHub.profileId = ""
        EngineHub.profileName = ""
        EngineHub.profileType = ""
        sessionStartEpoch = 0 // the pushed "disconnected" must not carry a stale started_at
        if (pushState) EngineHub.pushEvent("state", statusJson("disconnected"))
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        } finally {
            // Only after the whole pass: the engine Stop() above fires
            // onState("disconnected") synchronously — that is our teardown,
            // not a drop, and must not queue a reconnect.
            tearingDown = false
        }
    }

    /** The desktop Status shape the frontend renders. */
    private fun statusJson(state: String, error: String? = null): JSONObject =
        WisteriaBridge.staticStatusJson(
            state,
            if (state == "disconnected") "" else EngineHub.profileType,
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
        // stop the tunnel was the only way before.
        val disconnectIntent = PendingIntent.getService(
            this, 1,
            Intent(this, WisteriaVpnService::class.java).setAction(ACTION_DISCONNECT),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val text = when (state) {
            "connected" -> getString(R.string.status_connected)
            "connecting" -> getString(R.string.status_connecting)
            else -> getString(R.string.status_idle)
        }
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.ic_lock_lock)
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
