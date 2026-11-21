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
        // "Auto-connect on start" (settings.auto_connect) is the user's
        // explicit pick and wins over whatever was active last; an empty
        // value keeps the old activeId() behaviour.
        val auto = store.settings().optString("auto_connect").trim()
        val id = if (auto.isNotEmpty() && store.profile(auto) != null) auto else store.activeId()
        if (id.isEmpty()) return
        // Any profile type: the service itself branches to the wisteria
        // engine or the embedded sing-box. (Used to be wisteria-only, which
        // silently ignored vless/ss/… profiles with autostart enabled.)
        store.profile(id) ?: return
        if (auto.isNotEmpty() && auto != id) {
            EngineHub.appLog("автозапуск: профиль «$auto» из настроек не найден — беру активный")
        }

        try {
            context.startForegroundService(
                Intent(context, WisteriaVpnService::class.java)
                    .setAction(WisteriaVpnService.ACTION_CONNECT)
                    .putExtra(WisteriaVpnService.EXTRA_PROFILE_ID, id)
            )
        } catch (t: Throwable) {
            // Some OEM builds refuse the boot-time FGS start; a crash in
            // a BOOT_COMPLETED receiver would take the whole app down at
            // every boot. Log and skip — the user can connect manually.
            android.util.Log.w("WisteriaBoot", "autostart failed", t)
        }
    }
}
