package com.blindmap

import com.blindmap.net.socketUrl
import kotlin.test.Test
import kotlin.test.assertEquals

class SocketUrlTest {
    @Test
    fun firstConnectCarriesNoCredentials() {
        assertEquals(
            "wss://host/ws?room=ABC123&name=Alice",
            socketUrl("wss://host/ws", "ABC123", "Alice")
        )
    }

    @Test
    fun reconnectCarriesPlayerIdAndToken() {
        assertEquals(
            "wss://host/ws?room=ABC123&name=Alice&playerId=p1&token=s3cret",
            socketUrl("wss://host/ws", "ABC123", "Alice", playerId = "p1", reconnectToken = "s3cret")
        )
    }
}
