package org.wisteria.app

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.ServiceInfo
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
 * VPN frontend for the Go engine. The tunnel device is created here via
 * Builder.establish() and its fd handed to the engine; the Go side never
 * closes the fd, teardown always goes through this service. Every socket
 * the engine opens (server TCP dial, DNS resolver) is bound to the VPN
 * interface unless the Protect callback allows it out — that callback is
 * VpnService.protect().
 *
 * The service reads the profile from AndroidStore and pushes state/log
 * events through EngineHub, which the webview bridge serves to the
 * shared frontend.
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
    private val executor = Executors.newSingleThreadExecutor()

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
    }

    override fun onCreate() {
        super.onCreate()
        createChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_DISCONNECT -> executor.execute {
                teardown()
                stopSelf()
            }
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
        if (tun != null) return // already running

        val store = AndroidStore(this)
        val id = profileIdParam.ifEmpty { store.activeId() }
        val profile = store.profile(id)
        if (profile == null || profile.optString("type") != "wisteria") {
            EngineHub.appLog("подключение: профиль wisteria не найден")
            stopSelf()
            return
        }

        val builder = Builder()
            .addAddress(TUN_ADDRESS, TUN_PREFIX)
            .addRoute("0.0.0.0", 0)
            .addDnsServer(DNS)
            .setMtu(TUN_MTU)
            .setSession(profile.optString("name").ifEmpty { "Wisteria" })
        val fd = builder.establish()
        if (fd == null) {
            EngineHub.appLog("VpnService.establish() вернул null — согласие на VPN отозвано?")
            EngineHub.pushEvent("state", statusJson("disconnected"))
            stopSelf()
            return
        }
        tun = fd
        EngineHub.profileId = profile.optString("id")
        EngineHub.profileName = profile.optString("name")
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
        } catch (e: Exception) {
            EngineHub.coreLog("start failed: ${e.message}")
            EngineHub.pushEvent("state", statusJson("disconnected"))
            teardown()
            stopSelf()
        }
    }

    private fun teardown() {
        try {
            Mobile.stop()
        } catch (_: Throwable) {
        }
        tun?.let {
            try {
                it.close()
            } catch (_: Throwable) {
            }
        }
        tun = null
        EngineHub.profileId = ""
        EngineHub.profileName = ""
        EngineHub.pushEvent("state", statusJson("disconnected"))
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
    }

    /** The desktop Status shape the frontend renders. */
    private fun statusJson(state: String): JSONObject {
        val out = JSONObject()
            .put("state", state)
            .put("profile_id", EngineHub.profileId)
            .put("profile_name", EngineHub.profileName)
            .put("type", if (state == "disconnected") "" else "wisteria")
            .put("sys_proxy", false)
            .put("tun", false)
        return out
    }

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
