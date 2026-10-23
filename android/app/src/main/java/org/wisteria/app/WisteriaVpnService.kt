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
import org.json.JSONObject
import org.wisteria.mobile.Callback
import org.wisteria.mobile.Mobile

/**
 * VPN frontend for the Go engine. The tunnel device is created here via
 * Builder.establish() and its fd handed to the Go pump; the Go side never
 * closes the fd, teardown always goes through this service. Every socket
 * the engine opens (server TCP dial, DNS resolver) is bound to the VPN
 * interface unless the Protect callback allows it out — that callback is
 * VpnService.protect().
 */
class WisteriaVpnService : VpnService() {

    companion object {
        const val ACTION_CONNECT = "org.wisteria.app.CONNECT"
        const val ACTION_DISCONNECT = "org.wisteria.app.DISCONNECT"
        private const val CHANNEL_ID = "vpn"
        private const val NOTIF_ID = 1
        private const val TUN_ADDRESS = "10.8.0.2"
        private const val TUN_PREFIX = 24
        private const val TUN_MTU = 1400
        private const val DNS = "8.8.8.8"
    }

    private var tun: ParcelFileDescriptor? = null

    private val callback = object : Callback {
        override fun onLog(line: String?) {
            EngineState.addLog(line ?: "")
        }

        override fun onState(state: String?) {
            EngineState.set(state ?: "disconnected")
            updateNotification()
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
            ACTION_DISCONNECT -> {
                teardown()
                stopSelf()
            }
            else -> connect()
        }
        return START_NOT_STICKY
    }

    override fun onRevoke() {
        // The system (or the user in settings) revoked VPN consent.
        teardown()
        stopSelf()
    }

    override fun onDestroy() {
        teardown()
        super.onDestroy()
    }

    private fun connect() {
        if (tun != null) return // already running

        val profile = EngineState.profile(this)
        if (profile == null) {
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
            EngineState.addLog("VpnService.establish() вернул null — согласие на VPN отозвано?")
            EngineState.set("disconnected")
            stopSelf()
            return
        }
        tun = fd

        startForegroundWith()

        val server = profile.getJSONArray("servers").getJSONObject(0)
        val cfg = JSONObject().apply {
            put("endpoint", server.getString("endpoint"))
            put("pub", server.getString("pub"))
            put("priv", profile.getString("key"))
            put("name", profile.optString("name"))
        }.toString()

        EngineState.set("connecting")
        EngineState.addLog("запуск движка…")
        try {
            Mobile.start(cfg, fd.fd.toLong(), callback)
        } catch (e: Exception) {
            EngineState.addLog("start failed: ${e.message}")
            EngineState.set("disconnected")
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
        EngineState.set("disconnected")
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
    }

    private fun createChannel() {
        val nm = getSystemService(NotificationManager::class.java) ?: return
        nm.createNotificationChannel(
            NotificationChannel(CHANNEL_ID, "Wisteria VPN", NotificationManager.IMPORTANCE_LOW)
        )
    }

    private fun startForegroundWith() {
        ServiceCompat.startForeground(
            this, NOTIF_ID, buildNotification(),
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE
            else
                0
        )
    }

    private fun updateNotification() {
        val nm = getSystemService(NotificationManager::class.java) ?: return
        nm.notify(NOTIF_ID, buildNotification())
    }

    private fun buildNotification(): Notification {
        val contentIntent = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.ic_lock_lock)
            .setContentTitle("Wisteria")
            .setContentText(getString(statusRes(EngineState.current)))
            .setContentIntent(contentIntent)
            .setOngoing(true)
            .build()
    }

    private fun statusRes(state: String): Int = when (state) {
        "connecting" -> R.string.status_connecting
        "connected" -> R.string.status_connected
        "retrying" -> R.string.status_retrying
        else -> R.string.status_idle
    }
}
