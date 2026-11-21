package org.wisteria.app

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * The RU bypass list is a copied asset, refreshed by hand from the protocol
 * repo. A botched refresh (a stray hostname, a broken prefix length, a
 * leading-zero octet) must fail here instead of silently producing an
 * IpPrefix table the VpnService could not exclude — or worse, one that
 * excludes the wrong network. The asset ships both address families; the
 * service loads only IPv4 (the engine relays IPv4 and blackholes v6), so
 * parseIPv4() must reject every v6 line. parseList() itself needs API 33
 * IpPrefix objects, which is why the asset is validated through the same
 * parseIPv4() the loader uses.
 */
class RuRoutesTest {

    @Test
    fun parseIPv4AcceptsPlainDottedQuad() {
        assertArrayEquals(byteArrayOf(0, 0, 0, 0), RuRoutes.parseIPv4("0.0.0.0"))
        assertArrayEquals(byteArrayOf(1, 2, 3, 4), RuRoutes.parseIPv4("1.2.3.4"))
        assertArrayEquals(byteArrayOf(255.toByte(), 254.toByte(), 253.toByte(), 252.toByte()), RuRoutes.parseIPv4("255.254.253.252"))
    }

    @Test
    fun parseIPv4RejectsEverythingElse() {
        for (bad in listOf("", "1.2.3", "1.2.3.4.5", "256.1.1.1", "1.2.3.-1", "a.b.c.d", "01.2.3.4", "1.2.3.04", " 1.2.3.4", "::1", "1.2.3.4 ")) {
            assertNull("must be rejected: '$bad'", RuRoutes.parseIPv4(bad))
        }
    }

    @Test
    fun shippedAssetIsWellFormedForBothFamilies() {
        val file = assetFile()
        var v4 = 0
        var v6 = 0
        file.useLines { lines ->
            lines.forEachIndexed { index, raw ->
                val line = raw.trim()
                if (line.isEmpty() || line.startsWith("#")) return@forEachIndexed
                val where = "${file.name}:${index + 1}: $line"
                val slash = line.indexOf('/')
                assertTrue("not a CIDR: $where", slash > 0)
                val addr = line.substring(0, slash)
                val bits = line.substring(slash + 1).toIntOrNull()
                if (addr.contains(':')) {
                    // v6 entries are intentionally not loaded by the service,
                    // but a refresh must still leave them parseable.
                    assertTrue("bad IPv6 prefix length: $where", bits != null && bits in 0..128)
                    assertNull("v6 must be skipped by the IPv4 loader: $where", RuRoutes.parseIPv4(addr))
                    v6++
                } else {
                    assertTrue("bad IPv4 address: $where", RuRoutes.parseIPv4(addr) != null)
                    assertTrue("bad IPv4 prefix length: $where", bits != null && bits in 0..32)
                    v4++
                }
            }
        }
        // The desktop table has ~8.7k IPv4 prefixes; a collapse to a handful
        // means the asset was truncated or replaced by an error page.
        assertTrue("suspiciously small IPv4 route table: $v4 entries", v4 >= 8000)
        assertTrue("the asset is expected to ship v6 entries too, got $v6", v6 > 0)
    }

    /** The unit-test working directory is the module (android/app), but
     *  tolerate being launched from the project root as well. */
    private fun assetFile(): File {
        val candidates = listOf(
            File("src/main/assets/ru_networks.txt"),
            File("app/src/main/assets/ru_networks.txt"),
        )
        return candidates.firstOrNull { it.isFile }
            ?: throw AssertionError("ru_networks.txt not found; cwd=" + File(".").absolutePath)
    }
}
