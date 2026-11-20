package org.wisteria.app

import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import org.json.JSONObject
import org.wisteria.mobile.Mobile

/**
 * Quick-settings tile: connect / disconnect without opening the app.
 * onClick runs while SystemUI has this service bound — one of the Android
 * 12+ exemptions from the background foreground-service start restriction.
 * Every start is still wrapped: the fallback opens the app (which auto-
 * connects) instead of crashing on a exotic OEM refusal.
 */
class WisteriaTileService : TileService() {

    override fun onStartListening() {
        super.onStartListening()
        updateTile()
    }

    override fun onClick() {
        super.onClick()
        if (engineRunning()) {
            try {
                startVpn(Intent(this, WisteriaVpnService::class.java).setAction(WisteriaVpnService.ACTION_DISCONNECT))
            } catch (_: Throwable) {
            }
        } else {
            val id = AndroidStore(this).activeId()
            // No active profile, or the system VPN consent dialog is pending:
            // both need the activity — it picks the connect up on launch.
            if (id.isEmpty() || VpnService.prepare(this) != null) {
                openApp()
            } else {
                try {
                    startVpn(
                        Intent(this, WisteriaVpnService::class.java)
                            .setAction(WisteriaVpnService.ACTION_CONNECT)
                            .putExtra(WisteriaVpnService.EXTRA_PROFILE_ID, id)
                    )
                } catch (_: Throwable) {
                    openApp()
                }
            }
        }
        updateTile()
    }

    /** startForegroundService exists since API 26; on 24–25 a plain
     *  startService is legal even from the background. */
    private fun startVpn(intent: Intent) {
        if (Build.VERSION.SDK_INT >= 26) startForegroundService(intent) else startService(intent)
    }

    private fun openApp() {
        try {
            startActivity(
                Intent(this, MainActivity::class.java)
                    .putExtra(MainActivity.EXTRA_AUTO_CONNECT, true)
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            )
        } catch (_: Throwable) {
        }
    }

    private fun engineRunning(): Boolean = try {
        JSONObject(Mobile.status()).optString("state") in setOf("connecting", "connected", "retrying")
    } catch (_: Throwable) {
        false
    }

    private fun updateTile() {
        val tile = qsTile ?: return
        val running = engineRunning()
        tile.state = if (running) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
        tile.label = getString(R.string.app_name)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            tile.subtitle = getString(if (running) R.string.status_connected else R.string.status_idle)
        }
        tile.updateTile()
    }

    companion object {
        /** Ask SystemUI to re-bind and refresh the tile; safe from any
         *  component/thread (the VPN service calls it on every state push). */
        fun refresh(context: Context) {
            try {
                requestListeningState(context, ComponentName(context, WisteriaTileService::class.java))
            } catch (_: Throwable) {
            }
        }
    }
}