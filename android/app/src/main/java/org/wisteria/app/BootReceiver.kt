package org.wisteria.app

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

/**
 * Autostart (the desktop's "Start with Windows" counterpart): on boot,
 * when the toggle is on and there is an active wisteria profile, connect
 * right away. VPN consent persists across reboots, so no dialog is needed.
 */
class BootReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED) return
        val store = AndroidStore(context)
        if (!store.autostart()) return
        val id = store.activeId()
        if (id.isEmpty()) return
        val profile = store.profile(id) ?: return
        if (profile.optString("type") != "wisteria") return

        context.startForegroundService(
            Intent(context, WisteriaVpnService::class.java)
                .setAction(WisteriaVpnService.ACTION_CONNECT)
                .putExtra(WisteriaVpnService.EXTRA_PROFILE_ID, id)
        )
    }
}
