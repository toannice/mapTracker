package com.blindmap

import com.blindmap.net.healthUrlFrom
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class PrewarmTest {
    @Test
    fun derivesHttpsHealthUrlFromWssServerUrl() {
        assertEquals(
            "https://maptracker-c68n.onrender.com/healthz",
            healthUrlFrom("wss://maptracker-c68n.onrender.com/ws")
        )
    }

    @Test
    fun derivesHttpHealthUrlFromLocalWsUrl() {
        assertEquals("http://localhost:8080/healthz", healthUrlFrom("ws://localhost:8080/ws"))
    }

    @Test
    fun dropsQueryStringAndPath() {
        assertEquals(
            "https://host/healthz",
            healthUrlFrom("wss://host/ws?room=ABC123&name=Alice")
        )
    }

    @Test
    fun acceptsPlainHttpUrls() {
        assertEquals("https://host/healthz", healthUrlFrom("https://host/anything"))
    }

    @Test
    fun toleratesSurroundingWhitespace() {
        assertEquals("https://host/healthz", healthUrlFrom("  wss://host/ws  "))
    }

    // A malformed URL must skip the pre-warm rather than build a bogus request.
    @Test
    fun rejectsUnrecognisedInput() {
        assertNull(healthUrlFrom(""))
        assertNull(healthUrlFrom("maptracker.onrender.com/ws"))
        assertNull(healthUrlFrom("ftp://host/ws"))
        assertNull(healthUrlFrom("wss://"))
    }
}
