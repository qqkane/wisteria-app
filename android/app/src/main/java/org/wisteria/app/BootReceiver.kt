package org.wisteria.app

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

/**
 * Autostart (the desktop's "Start with Windows" counterpart): on boot,
 * when the toggle is on and there is an active profile, connect right
 * away. VPN consent persists across reboots, so no dialog is needed.
 */
class BootReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED) return
        val store = AndroidStore(context)
        if (!store.autostart()) return
        val id = store.activeId()
        if (id.isEmpty()) return
        // Any profile type: the service itself branches to the wisteria
        // engine or the embedded sing-box. (Used to be wisteria-only, which
        // silently ignored vless/ss/… profiles with autostart enabled.)
        store.profile(id) ?: return

        context.startForegroundService(
            Intent(context, WisteriaVpnService::class.java)
                .setAction(WisteriaVpnService.ACTION_CONNECT)
                .putExtra(WisteriaVpnService.EXTRA_PROFILE_ID, id)
        )
    }
}
