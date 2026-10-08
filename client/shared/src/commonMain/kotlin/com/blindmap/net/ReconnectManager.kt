package com.blindmap.net

import com.blindmap.state.ConnState
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlin.math.min
import kotlin.math.pow

class ReconnectManager(private val wsClient: WebSocketClient) {
    val connState = MutableStateFlow(ConnState.Connecting)

    suspend fun reconnect(
        playerId: String,
        reconnectToken: String,
        serverUrl: String,
        roomCode: String,
        playerName: String
    ) {
        connState.value = ConnState.Reconnecting
        repeat(5) { attempt ->
            val delayMs = min(2.0.pow(attempt).toLong() * 1000L, 30_000L)
            delay(delayMs)
            try {
                wsClient.connect(serverUrl, roomCode, playerName, playerId, reconnectToken)
                connState.value = ConnState.Connected
                return
            } catch (e: Exception) {
                // retry
            }
        }
        connState.value = ConnState.Failed
    }
}
