package org.wisteria.app

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Base64

/**
 * Subscription response handling: the two headers every client shows
 * (Subscription-Userinfo, Profile-Title) and the HTML-page guard that stops
 * a captive portal from wiping a working subscription.
 */
class SubscriptionMetaTest {

    @Test
    fun userinfoIsParsedIntoTheStoredSub() {
        val sub = JSONObject().put("name", "old")
        SubscriptionMeta.apply(
            sub,
            mapOf("Subscription-Userinfo" to "upload=100; download=200; total=1000; expire=1700000000"),
        )
        val u = sub.getJSONObject("userinfo")
        assertEquals(100L, u.getLong("upload"))
        assertEquals(200L, u.getLong("download"))
        assertEquals(1000L, u.getLong("total"))
        assertEquals(1700000000L, u.getLong("expire"))
    }

    @Test
    fun headerNamesAreCaseInsensitive() {
        val sub = JSONObject()
        SubscriptionMeta.apply(sub, mapOf("subscription-userinfo" to "upload=1;download=2;total=3"))
        assertEquals(1L, sub.getJSONObject("userinfo").getLong("upload"))
        assertEquals(2L, sub.getJSONObject("userinfo").getLong("download"))
        assertEquals(3L, sub.getJSONObject("userinfo").getLong("total"))
        assertFalse(sub.getJSONObject("userinfo").has("expire"))
    }

    @Test
    fun malformedPairsAreIgnoredAndAllGarbageStoresNothing() {
        val partly = JSONObject()
        SubscriptionMeta.apply(
            partly,
            mapOf("Subscription-Userinfo" to "upload=10; nonsense; =5; total=x; download=7"),
        )
        val u = partly.getJSONObject("userinfo")
        assertEquals(10L, u.getLong("upload"))
        assertEquals(7L, u.getLong("download"))
        assertEquals(2, u.length())

        val garbage = JSONObject()
        SubscriptionMeta.apply(garbage, mapOf("Subscription-Userinfo" to "hello; world=; ="))
        assertFalse(garbage.has("userinfo"))
    }

    @Test
    fun plainProfileTitleRenamesTheSub() {
        val sub = JSONObject().put("name", "my.server.example")
        SubscriptionMeta.apply(sub, mapOf("Profile-Title" to "  My Subscription  "))
        assertEquals("My Subscription", sub.getString("name"))
    }

    @Test
    fun base64ProfileTitleKeepsUtf8() {
        val sub = JSONObject().put("name", "old")
        val title = "Моя подписка"
        val encoded = Base64.getEncoder().encodeToString(title.toByteArray(Charsets.UTF_8))
        SubscriptionMeta.apply(sub, mapOf("Profile-Title" to "base64:$encoded"))
        assertEquals(title, sub.getString("name"))
    }

    @Test
    fun brokenBase64TitleLeavesTheOldName() {
        val sub = JSONObject().put("name", "old")
        SubscriptionMeta.apply(sub, mapOf("Profile-Title" to "base64:!!!not-base64!!!"))
        assertEquals("old", sub.getString("name"))
    }

    @Test
    fun emptyTitleIsIgnored() {
        val sub = JSONObject().put("name", "old")
        SubscriptionMeta.apply(sub, mapOf("Profile-Title" to "   "))
        assertEquals("old", sub.getString("name"))
    }

    @Test
    fun htmlErrorPagesAreRecognized() {
        assertTrue(SubscriptionMeta.looksLikeHtml("<!DOCTYPE html><html><body>404</body></html>"))
        assertTrue(SubscriptionMeta.looksLikeHtml("  <html lang=en><h1>Expired</h1>"))
        assertTrue(SubscriptionMeta.looksLikeHtml("<head><title>x</title></head>"))
        assertTrue(SubscriptionMeta.looksLikeHtml("<body><p>captive portal</p></body>"))
    }

    @Test
    fun linkListsAndBase64BlobsAreNotHtml() {
        assertFalse(SubscriptionMeta.looksLikeHtml("vless://a@b:443#node\nss://x@1.2.3.4:8388"))
        assertFalse(SubscriptionMeta.looksLikeHtml("dmxlc3M6Ly9hQGI6NDQz"))
        assertFalse(SubscriptionMeta.looksLikeHtml(""))
        // An HTML mention deep in a long body must not trip the guard: only
        // the first 2 KB are inspected (the marker cannot be a link list).
        val filler = "vless://a@b:443\n".repeat(400)
        assertFalse(SubscriptionMeta.looksLikeHtml(filler + "<html>"))
    }
}
