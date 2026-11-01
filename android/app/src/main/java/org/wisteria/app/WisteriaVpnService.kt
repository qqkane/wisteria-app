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

    private var tun: ParcelFileDescriptor? = null
    private var boxTun: ParcelFileDescriptor? = null
    private var boxNetCallback: ConnectivityManager.NetworkCallback? = null
    private val executor = Executors.newSingleThreadExecutor()
    private var engineIsBox = false

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
                WisteriaBridgeHolder.instance?.onConnectedCheckIP()
            }
            updateNotification(mapped)
        }

        override fun protect(fd: Long): Boolean =
            this@WisteriaVpnService.protect(fd.toInt())

        override fun onOpenTun(optionsJson: String?): Int {
            val opts = try { JSONObject(optionsJson ?: "{}") } catch (_: Exception) { JSONObject() }
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
            builder.addDnsServer(DNS)
            val pfd = builder.establish() ?: return -1
            boxTun = pfd
            return pfd.fd
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
                teardown()
                stopSelf()
            }, "wisteria-disconnect").start()
            else -> {
                val id = intent?.getStringExtra(EXTRA_PROFILE_ID).orEmpty()
                executor.execute { connect(id) }
            }
        }
        return START_NOT_STICKY
    }

    override fun onRevoke() {
        // The system (or the user in settings) revoked VPN consent.
        executor.execute {
            teardown()
            stopSelf()
        }
    }

    override fun onDestroy() {
        executor.execute {
            teardown()
        }
        executor.shutdown()
        super.onDestroy()
    }

    private fun connect(profileIdParam: String) {
        val myEpoch = synchronized(stateLock) {
            if (tun != null || engineIsBox) return // already running
            teardownEpoch
        }

        val store = AndroidStore(this)
        val id = profileIdParam.ifEmpty { store.activeId() }
        val profile = store.profile(id)
            ?: run {
                EngineHub.appLog("подключение: профиль не найден")
                stopSelf()
                return
            }

        if (profile.optString("type") == "wisteria") {
            startWisteria(profile, myEpoch)
        } else {
            startBox(profile, myEpoch)
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
            .addDnsServer(DNS)
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
        startForegroundWith()

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
            }
        } catch (e: Exception) {
            EngineHub.coreLog("start failed: ${e.message}")
            // Show the failure instead of silently swallowing it: the VPN
            // screen renders status.error in the red box (v0.4.2's
            // "connecting forever" looked exactly like a swallowed error).
            teardown(pushState = false)
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
        startForegroundWith()
        EngineHub.pushEvent("state", statusJson("connecting"))
        try {
            Mobile.boxStart(config, callback)
            registerBoxNetworkMonitor()
            if (epochChanged(myEpoch)) {
                EngineHub.appLog("отмена пришла во время запуска — останавливаю")
                teardown(pushState = true)
                stopSelf()
            }
        } catch (e: Exception) {
            EngineHub.coreLog("sing-box: ${e.message}")
            // See startWisteria: surface the failure on the VPN screen.
            teardown(pushState = false)
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
        if (pushState) EngineHub.pushEvent("state", statusJson("disconnected"))
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
    }

    /** The desktop Status shape the frontend renders. */
    private fun statusJson(state: String, error: String? = null): JSONObject =
        WisteriaBridge.staticStatusJson(state, if (state == "disconnected") "" else EngineHub.profileType, error)

    private fun createChannel() {
        val nm = getSystemService(NotificationManager::class.java) ?: return
        nm.createNotificationChannel(
            NotificationChannel(CHANNEL_ID, "Wisteria VPN", NotificationManager.IMPORTANCE_LOW)
        )
    }

    private fun startForegroundWith() {
        ServiceCompat.startForeground(
            this, NOTIF_ID, buildNotification("connecting"),
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
