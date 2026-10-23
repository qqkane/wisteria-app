package org.wisteria.app

import android.app.Activity
import android.content.Intent
import android.net.VpnService
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import org.json.JSONArray
import org.wisteria.mobile.Mobile

class MainActivity : AppCompatActivity() {

    companion object {
        private const val REQ_VPN = 1
    }

    private val handler = Handler(Looper.getMainLooper())

    private val listener = object : EngineState.Listener {
        override fun onChanged(state: String) {
            handler.post { renderState() }
        }

        override fun onLog(line: String) {
            handler.post { appendLog(line) }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        findViewById<Button>(R.id.toggleButton).setOnClickListener { onToggle() }
        findViewById<Button>(R.id.addButton).setOnClickListener { onAddProfile() }
    }

    override fun onResume() {
        super.onResume()
        EngineState.addListener(listener)
        renderState()
        renderProfile()
        findViewById<TextView>(R.id.logText).text = EngineState.logText()
    }

    override fun onPause() {
        super.onPause()
        EngineState.removeListener(listener)
    }

    private fun onToggle() {
        if (EngineState.current in activeStates) {
            stopVpn()
        } else {
            beginConnect()
        }
    }

    private fun beginConnect() {
        if (EngineState.profile(this) == null) {
            toast(R.string.no_profile)
            return
        }
        val consent = VpnService.prepare(this)
        if (consent != null) {
            startActivityForResult(consent, REQ_VPN)
        } else {
            launchVpn()
        }
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == REQ_VPN && resultCode == Activity.RESULT_OK) {
            launchVpn()
        }
    }

    private fun launchVpn() {
        startService(
            Intent(this, WisteriaVpnService::class.java)
                .setAction(WisteriaVpnService.ACTION_CONNECT)
        )
    }

    private fun stopVpn() {
        startService(
            Intent(this, WisteriaVpnService::class.java)
                .setAction(WisteriaVpnService.ACTION_DISCONNECT)
        )
    }

    /**
     * Parse the link(s) pasted by the user. v1 takes the first entry and
     * requires a client key: invite-only links (no key) cannot build the
     * config, so they are rejected with an explicit message instead of a
     * silent half-working profile.
     */
    private fun onAddProfile() {
        val raw = findViewById<EditText>(R.id.linkInput).text.toString().trim()
        if (raw.isEmpty()) return

        val json = try {
            Mobile.parseLinks(raw)
        } catch (e: Exception) {
            EngineState.addLog("link: ${e.message}")
            toast(R.string.bad_link)
            return
        }

        val arr = JSONArray(json)
        if (arr.length() == 0) {
            toast(R.string.bad_link)
            return
        }
        val profile = arr.getJSONObject(0)
        if (profile.optString("key").isEmpty()) {
            toast(R.string.invite_unsupported)
            return
        }
        val servers = profile.optJSONArray("servers")
        if (servers == null || servers.length() == 0 ||
            servers.getJSONObject(0).optString("endpoint").isEmpty()
        ) {
            toast(R.string.bad_link)
            return
        }

        EngineState.saveProfile(this, profile)
        findViewById<EditText>(R.id.linkInput).setText("")
        toast(R.string.added_ok)
        renderProfile()
    }

    private fun renderState() {
        findViewById<TextView>(R.id.statusText).setText(statusRes(EngineState.current))
        findViewById<Button>(R.id.toggleButton).setText(
            if (EngineState.current in activeStates) R.string.disconnect else R.string.connect
        )
    }

    private fun renderProfile() {
        val profile = EngineState.profile(this)
        findViewById<TextView>(R.id.profileText).text = if (profile == null) {
            getString(R.string.no_profile)
        } else {
            getString(R.string.profile_label, profile.optString("name"))
        }
        findViewById<TextView>(R.id.myKeyText).text = if (profile == null) {
            ""
        } else {
            try {
                Mobile.myPublicKey(profile.getString("key"))
            } catch (_: Exception) {
                "-"
            }
        }
    }

    private fun appendLog(line: String) {
        val view = findViewById<TextView>(R.id.logText)
        val text = view.text.toString()
        view.text = if (text.isEmpty()) line else "$text\n$line"
    }

    private fun statusRes(state: String): Int = when (state) {
        "connecting" -> R.string.status_connecting
        "connected" -> R.string.status_connected
        "retrying" -> R.string.status_retrying
        "disconnected" -> R.string.status_disconnected
        else -> R.string.status_idle
    }

    private fun toast(res: Int) {
        Toast.makeText(this, res, Toast.LENGTH_LONG).show()
    }
}

private val activeStates = setOf("connecting", "connected", "retrying")
