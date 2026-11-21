package org.wisteria.app

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Rules that broke real sessions before and are pure enough to pin down on
 * the JVM: the route-list classifier (a hex-letter regex used to turn
 * cafe.de into an ip_cidr sing-box refuses), the DNS resolver pair (one
 * resolver made the proxy's own name resolve through the tunnel) and the
 * hysteria2 password decode (a second URLDecoder turned '+' into space).
 */
class SingboxLinksTest {

    // ---- route list classification ----

    @Test
    fun hexOnlyDomainsStayDomainSuffix() {
        for (domain in listOf("cafe.de", "bbc.de", "feed.ac", "dead.beef.example", "abc.def")) {
            assertEquals(domain, SingboxLinks.parseRouteList(domain).single().value)
            assertEquals(
                "must not be emitted as ip_cidr: $domain",
                SingboxLinks.RouteKind.DOMAIN,
                SingboxLinks.parseRouteList(domain).single().kind,
            )
        }
    }

    @Test
    fun realAddressesAreCidr() {
        val entries = SingboxLinks.parseRouteList(
            "1.2.3.4\n10.0.0.0/8\n2a14:e700::/124\n2001:db8::1\n::ffff:1.2.3.4"
        )
        assertTrue(entries.all { it.kind == SingboxLinks.RouteKind.CIDR })
        assertEquals(listOf("1.2.3.4", "10.0.0.0/8", "2a14:e700::/124", "2001:db8::1", "::ffff:1.2.3.4"), entries.map { it.value })
    }

    @Test
    fun badPrefixLengthIsRejectedEvenForValidAddress() {
        assertFalse(SingboxLinks.isCidr("10.0.0.0/33"))
        assertFalse(SingboxLinks.isCidr("2a14:e700::/129"))
        assertFalse(SingboxLinks.isCidr("10.0.0.0/"))
        assertFalse(SingboxLinks.isCidr("10.0.0.0/8/9"))
        assertTrue(SingboxLinks.isCidr("0.0.0.0/0"))
    }

    @Test
    fun leadingZeroOctetsAreNotIPv4() {
        // Go's net.ParseIP rejects them; emitting 010.1.1.1 as ip_cidr makes
        // sing-box refuse the whole config.
        assertFalse(SingboxLinks.isCidr("010.1.1.1"))
        assertFalse(SingboxLinks.isIPv4("1.2.3.04"))
        assertTrue(SingboxLinks.isIPv4("1.2.3.4"))
    }

    @Test
    fun geoAndWildcardSpellingsAreNormalized() {
        val entries = SingboxLinks.parseRouteList(
            """
            # comment
            geoip:ru
            geosite:category-ads-all
            *.example.com   // xray suffix form
            Ads.Example.COM ; 5.6.7.8
            """.trimIndent()
        )
        assertEquals(SingboxLinks.RouteEntry("geoip-ru", SingboxLinks.RouteKind.GEOIP), entries[0])
        assertEquals(SingboxLinks.RouteEntry("geosite-category-ads-all", SingboxLinks.RouteKind.GEOSITE), entries[1])
        assertEquals(SingboxLinks.RouteEntry("example.com", SingboxLinks.RouteKind.DOMAIN), entries[2])
        assertEquals(SingboxLinks.RouteEntry("ads.example.com", SingboxLinks.RouteKind.DOMAIN), entries[3])
        assertEquals(SingboxLinks.RouteEntry("5.6.7.8", SingboxLinks.RouteKind.CIDR), entries[4])
    }

    // ---- percent decoding ----

    @Test
    fun plusIsNotSpace() {
        // URI encoding, not form encoding: '+' is a legal password character.
        assertEquals("a+b", SingboxLinks.percentDecode("a+b"))
        assertEquals("a+b", SingboxLinks.percentDecode("a%2Bb"))
        assertEquals("a b", SingboxLinks.percentDecode("a%20b"))
        assertEquals("100%", SingboxLinks.percentDecode("100%25"))
        assertEquals("плюс+", SingboxLinks.percentDecode("%D0%BF%D0%BB%D1%8E%D1%81+"))
        // An incomplete escape is left as-is, not dropped.
        assertEquals("50%", SingboxLinks.percentDecode("50%"))
        assertEquals("50%zz", SingboxLinks.percentDecode("50%zz"))
    }

    // ---- DNS ----

    @Test
    fun dnsServersAlwaysHaveDirectResolver() {
        val servers = SingboxLinks.dnsServers(JSONObject())
        assertEquals(2, servers.length())
        val remote = servers.getJSONObject(0)
        val direct = servers.getJSONObject(1)
        assertEquals(SingboxLinks.DNS_REMOTE_TAG, remote.getString("tag"))
        assertEquals("proxy", remote.getString("detour"))
        assertEquals("udp", remote.getString("type"))
        assertEquals("8.8.8.8", remote.getString("server"))
        assertEquals(SingboxLinks.DNS_DIRECT_TAG, direct.getString("tag"))
        assertEquals("direct", direct.getString("detour"))
        assertEquals("1.1.1.1", direct.getString("server"))
    }

    @Test
    fun dohKeepsHostPortAndPathSeparate() {
        val o = SingboxLinks.parseDnsServer("https://dns.google:8443/dns-query", "remote", "proxy")
        assertEquals("https", o.getString("type"))
        assertEquals("dns.google", o.getString("server"))
        assertEquals(8443, o.getInt("server_port"))
        assertEquals("/dns-query", o.getString("path"))
        assertEquals("proxy", o.getString("detour"))
    }

    @Test
    fun dohWithoutPathGetsDefaultPath() {
        val o = SingboxLinks.parseDnsServer("https://cloudflare-dns.com")
        assertEquals("cloudflare-dns.com", o.getString("server"))
        assertEquals("/dns-query", o.getString("path"))
        assertFalse(o.has("server_port"))
    }

    @Test
    fun dotAndTcpAndUdpAndIpv6AreParsed() {
        val dot = SingboxLinks.parseDnsServer("tls://dns.example:853")
        assertEquals("tls", dot.getString("type"))
        assertEquals("dns.example", dot.getString("server"))
        assertEquals(853, dot.getInt("server_port"))

        val bare = SingboxLinks.parseDnsServer("8.8.4.4")
        assertEquals("udp", bare.getString("type"))
        assertEquals("8.8.4.4", bare.getString("server"))

        val v6 = SingboxLinks.parseDnsServer("[2606:4700:4700::1111]:53")
        assertEquals("2606:4700:4700::1111", v6.getString("server"))
        assertEquals(53, v6.getInt("server_port"))
    }

    @Test
    fun localResolverFallsBackToProxiedUdp() {
        // "local" would mean the system resolver, which inside the TUN is the
        // resolver we are replacing (and a classic loop). It falls back to the
        // same proxied UDP default as an empty value.
        val o = SingboxLinks.parseDnsServer("local", "d", null)
        assertEquals("udp", o.getString("type"))
        assertEquals("8.8.8.8", o.getString("server"))
    }

    // ---- generated sing-box config ----

    private fun cfg(link: String, settings: JSONObject = JSONObject()): JSONObject =
        JSONObject(SingboxLinks.buildConfig(link, "test", settings))

    @Test
    fun proxyOutboundResolvesThroughDirectDns() {
        val c = cfg("vless://11111111-2222-3333-4444-555555555555@example.com:443?security=tls&sni=example.com")
        assertEquals(SingboxLinks.DNS_DIRECT_TAG, c.getJSONArray("outbounds").getJSONObject(0).getString("domain_resolver"))
        assertEquals(SingboxLinks.DNS_REMOTE_TAG, c.getJSONObject("dns").getString("final"))
        assertEquals(2, c.getJSONObject("dns").getJSONArray("servers").length())
    }

    @Test
    fun hysteria2PasswordIsDecodedExactlyOnce() {
        val c = cfg("hysteria2://pass+word%2Bx@hy.example.com:443?sni=hy.example.com#HY")
        val out = c.getJSONArray("outbounds").getJSONObject(0)
        assertEquals("hysteria2", out.getString("type"))
        // '+' stays '+', %2B becomes '+': both meant one literal plus.
        assertEquals("pass+word+x", out.getString("password"))
    }

    @Test
    fun routeListsLandInSeparateRulesWithRightKinds() {
        val s = JSONObject()
            .put("route_direct", "cafe.de\n1.2.3.0/24")
            .put("route_block", "ads.example.com\ngeosite:category-ads-all")
        val c = cfg("vless://11111111-2222-3333-4444-555555555555@example.com:443", s)
        val rules = c.getJSONObject("route").getJSONArray("rules")
        var direct: JSONObject? = null
        var block: JSONObject? = null
        for (i in 0 until rules.length()) {
            val r = rules.getJSONObject(i)
            if (r.optString("outbound") == "direct") direct = r
            if (r.optString("outbound") == "block") block = r
        }
        assertEquals("[\"cafe.de\"]", direct!!.getJSONArray("domain_suffix").toString())
        assertEquals("[\"1.2.3.0/24\"]", direct.getJSONArray("ip_cidr").toString())
        assertEquals("[\"ads.example.com\"]", block!!.getJSONArray("domain_suffix").toString())
        // The geosite entry became a rule-set reference, not a domain.
        assertEquals(1, block.getJSONArray("rule_set").length())
    }

    @Test
    fun tunStackAndStrictRouteComeFromSettings() {
        val s = JSONObject().put("tun_stack", "gvisor").put("tun_strict_route", true).put("log_level", "debug")
        val c = cfg("vless://11111111-2222-3333-4444-555555555555@example.com:443", s)
        val tun = c.getJSONArray("inbounds").getJSONObject(0)
        assertEquals("gvisor", tun.getString("stack"))
        assertTrue(tun.getBoolean("strict_route"))
        assertEquals("debug", c.getJSONObject("log").getString("level"))
    }

    @Test
    fun unknownTunStackFallsBackToMixed() {
        val c = cfg("vless://11111111-2222-3333-4444-555555555555@example.com:443",
            JSONObject().put("tun_stack", "bogus"))
        assertEquals("mixed", c.getJSONArray("inbounds").getJSONObject(0).getString("stack"))
    }

    @Test
    fun cacheFileOnlyAddedWhenPathGiven() {
        val base = cfg("vless://11111111-2222-3333-4444-555555555555@example.com:443")
        assertFalse(base.has("experimental"))
        val withCache = JSONObject(
            SingboxLinks.buildConfig(
                "vless://11111111-2222-3333-4444-555555555555@example.com:443",
                "test", null, "/data/cache.db",
            )
        )
        assertEquals("/data/cache.db", withCache.getJSONObject("experimental").getJSONObject("cache_file").getString("path"))
    }

    @Test
    fun plainLinkParsesToExpectedProfile() {
        val p = SingboxLinks.parseProfile("trojan://p%2Bss@t.example.com:8443?sni=t.example.com#My%20Node")
        assertEquals("My Node", p!!.getString("name"))
        assertEquals("trojan", p.getString("type"))
        assertEquals("t.example.com", p.getString("host"))
        assertEquals(8443, p.getInt("port"))
        assertEquals("trojan://p%2Bss@t.example.com:8443?sni=t.example.com#My%20Node", p.getString("raw"))
    }

    @Test
    fun malformedLinksReturnNullInsteadOfThrowing() {
        assertNull(SingboxLinks.parseProfile("vless://"))
        assertNull(SingboxLinks.parseProfile("vmess://not-base64!!!"))
        assertNull(SingboxLinks.parseProfile("https://example.com/sub"))
        assertFalse(SingboxLinks.handles("wisteria://abc"))
        assertTrue(SingboxLinks.handles("hy2://x"))
    }
}
