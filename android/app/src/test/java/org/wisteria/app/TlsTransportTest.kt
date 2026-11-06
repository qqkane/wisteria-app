package org.wisteria.app

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * TLS transport (wave 4) rules, mirrored from the protocol's
 * internal/config loader and the desktop's internal/core/profile.go:
 * the transport spelling, the RFC 1123 SNI sanity checks (no IP literals —
 * RFC 6066), the sni/insecure-without-tls combo rejection, the tcp→absent
 * normalization, the bundle-row → profile mirroring and the engine row
 * build for manual profiles.
 *
 * Runs on the JVM: TlsTransport touches only org.json (the real artifact
 * is a test dependency; the android.jar copy is stubbed locally).
 */
class TlsTransportTest {

    // ---- validateTransport ----

    @Test
    fun transportAcceptsTcpTlsAndEmpty() {
        assertNull(TlsTransport.validateTransport(""))
        assertNull(TlsTransport.validateTransport("tcp"))
        assertNull(TlsTransport.validateTransport("tls"))
    }

    @Test
    fun transportRejectsTyposCaseSensitively() {
        // case-sensitive like the core loader: "TLS" must not silently
        // fall back to tcp and change what lands on the wire
        assertNotNull(TlsTransport.validateTransport("TLS"))
        assertNotNull(TlsTransport.validateTransport("quic"))
        assertNotNull(TlsTransport.validateTransport(" tls "))
    }

    // ---- validSni ----

    @Test
    fun sniAcceptsSaneHostnames() {
        assertNull(TlsTransport.validSni(""))
        assertNull(TlsTransport.validSni("cdn.example.com"))
        assertNull(TlsTransport.validSni("a-b.example"))
        assertNull(TlsTransport.validSni("A1.b2.C3"))
        assertNull(TlsTransport.validSni("x".repeat(63))) // one max-length label
    }

    @Test
    fun sniRejectsIpLiterals() {
        assertNotNull(TlsTransport.validSni("1.2.3.4"))
        assertNotNull(TlsTransport.validSni("10.0.0.1"))
        assertNotNull(TlsTransport.validSni("::1"))
        assertNotNull(TlsTransport.validSni("2001:db8::1"))
    }

    @Test
    fun sniRejectsUnderscoresEmptyLabelsAndHyphenEdges() {
        assertNotNull(TlsTransport.validSni("cdn_zone.example")) // underscore
        assertNotNull(TlsTransport.validSni(".example.com")) // empty label
        assertNotNull(TlsTransport.validSni("example.com.")) // empty last label
        assertNotNull(TlsTransport.validSni("-cdn.example")) // leading hyphen
        assertNotNull(TlsTransport.validSni("cdn-.example")) // trailing hyphen
        assertNotNull(TlsTransport.validSni("cdn!.example")) // bad char
        assertNotNull(TlsTransport.validSni("x".repeat(64) + ".example")) // label > 63
        assertNotNull(TlsTransport.validSni("${"x".repeat(63)}.${"y".repeat(63)}.${"z".repeat(63)}.${"w".repeat(63)}")) // > 253
    }

    // ---- normalizeTls ----

    @Test
    fun tcpNormalizesOntoAbsentDefault() {
        val p = JSONObject("""{"type":"wisteria","transport":"tcp"}""")
        TlsTransport.normalizeTls(p)
        assertFalse(p.has("transport"))
        assertFalse(p.has("tls_sni"))
        assertFalse(p.has("tls_insecure"))
    }

    @Test
    fun absentTransportStaysBareTcp() {
        val p = JSONObject("""{"type":"wisteria"}""")
        TlsTransport.normalizeTls(p)
        assertFalse(p.has("transport"))
    }

    @Test
    fun tlsProfileKeepsCanonicalFields() {
        val p = JSONObject("""{"type":"wisteria","transport":"tls","tls_sni":"  cdn.example  ","tls_insecure":true}""")
        TlsTransport.normalizeTls(p)
        assertEquals("tls", p.getString("transport"))
        assertEquals("cdn.example", p.getString("tls_sni")) // trimmed
        assertTrue(p.getBoolean("tls_insecure"))
    }

    @Test
    fun sniOrInsecureWithoutTlsIsRejected() {
        val sniOnly = JSONObject("""{"type":"wisteria","tls_sni":"cdn.example"}""")
        try {
            TlsTransport.normalizeTls(sniOnly)
            throw AssertionError("sni without tls must fail")
        } catch (e: IllegalArgumentException) {
            assertTrue(e.message!!.contains("transport"))
        }
        val insecureOnly = JSONObject("""{"type":"wisteria","tls_insecure":true}""")
        try {
            TlsTransport.normalizeTls(insecureOnly)
            throw AssertionError("tls_insecure without tls must fail")
        } catch (e: IllegalArgumentException) {
            assertTrue(e.message!!.contains("transport"))
        }
    }

    @Test
    fun badTransportAndBadSniAreRejected() {
        val badTr = JSONObject("""{"type":"wisteria","transport":"quic"}""")
        try {
            TlsTransport.normalizeTls(badTr)
            throw AssertionError("typo'd transport must fail")
        } catch (e: IllegalArgumentException) {
            assertTrue(e.message!!.contains("transport"))
        }
        val ipSni = JSONObject("""{"type":"wisteria","transport":"tls","tls_sni":"1.2.3.4"}""")
        try {
            TlsTransport.normalizeTls(ipSni)
            throw AssertionError("IP SNI must fail")
        } catch (e: IllegalArgumentException) {
            assertTrue(e.message!!.contains("IP"))
        }
    }

    @Test
    fun nonWisteriaProfilesAreUntouched() {
        // the sing-box family keeps its transport in Fields; TLS is wisteria-only
        val p = JSONObject("""{"type":"vless","transport":"tcp","tls_sni":"1.2.3.4","tls_insecure":true}""")
        TlsTransport.normalizeTls(p)
        assertEquals("tcp", p.getString("transport"))
        assertEquals("1.2.3.4", p.getString("tls_sni"))
        assertTrue(p.getBoolean("tls_insecure"))
    }

    // ---- applyPrimaryTls ----

    @Test
    fun primaryTlsRowMirrorsOntoProfile() {
        val p = JSONObject("""{"type":"wisteria","host":"srv.example","port":443}""")
        val servers = JSONArray(
            """[{"endpoint":"srv.example:443","pub":"pk=","transport":"tls","tls_sni":"cdn.example","tls_insecure":true}]"""
        )
        TlsTransport.applyPrimaryTls(p, servers)
        assertEquals("tls", p.getString("transport"))
        assertEquals("cdn.example", p.getString("tls_sni"))
        assertTrue(p.getBoolean("tls_insecure"))
    }

    @Test
    fun tcpPrimaryRowLeavesProfileBare() {
        val p = JSONObject("""{"type":"wisteria","host":"srv.example","port":443}""")
        val servers = JSONArray("""[{"endpoint":"srv.example:443","pub":"pk="}]""")
        TlsTransport.applyPrimaryTls(p, servers)
        assertFalse(p.has("transport"))
        // old links parse to the exact same profile shape as before wave 4
        assertFalse(p.has("tls_sni"))
        assertFalse(p.has("tls_insecure"))
    }

    // ---- engineServers (store → Start config) ----

    @Test
    fun manualTlsProfileBuildsSingleEngineRow() {
        val p = JSONObject(
            """{"type":"wisteria","host":"srv.example","port":48200,"client_priv":"sk=","pub":"pk=","transport":"tls","tls_sni":"cdn.example","tls_insecure":true}"""
        )
        val rows = TlsTransport.engineServers(p)
        assertEquals(1, rows.length())
        val row = rows.getJSONObject(0)
        assertEquals("srv.example:48200", row.getString("endpoint"))
        assertEquals("pk=", row.getString("pub"))
        assertEquals("tls", row.getString("transport"))
        assertEquals("cdn.example", row.getString("tls_sni"))
        assertTrue(row.getBoolean("tls_insecure"))
    }

    @Test
    fun manualTcpProfileRowOmitsTlsKeys() {
        // omitempty semantics: a bare-tcp row keeps the pre-wave-4 shape
        val p = JSONObject("""{"type":"wisteria","host":"srv.example","port":48200,"pub":"pk="}""")
        val row = TlsTransport.engineServers(p).getJSONObject(0)
        assertEquals("srv.example:48200", row.getString("endpoint"))
        assertEquals("pk=", row.getString("pub"))
        assertFalse(row.has("transport"))
        assertFalse(row.has("tls_sni"))
        assertFalse(row.has("tls_insecure"))
    }

    @Test
    fun storedBundleRowsPassThroughUntouched() {
        val rows = JSONArray(
            """[{"endpoint":"a:1","pub":"k1=","transport":"tls","tls_sni":"cdn.example"},{"endpoint":"b:2","pub":"k2="}]"""
        )
        val p = JSONObject("""{"type":"wisteria","host":"a","port":1}""").put("servers", rows)
        val out = TlsTransport.engineServers(p)
        assertEquals(2, out.length())
        assertEquals("a:1", out.getJSONObject(0).getString("endpoint"))
        assertEquals("tls", out.getJSONObject(0).getString("transport"))
        assertEquals("b:2", out.getJSONObject(1).getString("endpoint"))
        assertFalse(out.getJSONObject(1).has("transport"))
    }
}
