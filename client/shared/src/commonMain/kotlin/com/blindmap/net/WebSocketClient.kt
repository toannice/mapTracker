package com.blindmap.net

import com.blindmap.protocol.Envelope
import io.ktor.client.HttpClient
import io.ktor.client.plugins.websocket.WebSockets
import io.ktor.client.plugins.websocket.webSocketSession
import io.ktor.websocket.Frame
import io.ktor.websocket.WebSocketSession
import io.ktor.websocket.close
import io.ktor.websocket.readText
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.buildJsonObject

class WebSocketClient {
    private val json = Json { ignoreUnknownKeys = true }
    private val client = HttpClient { install(WebSockets) }
    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())

    private val _incoming = MutableSharedFlow<Envelope>(extraBufferCapacity = 64)
    val incoming: SharedFlow<Envelope> = _incoming.asSharedFlow()

    private val _onClosed = MutableSharedFlow<Unit>(extraBufferCapacity = 1)
    val onClosed: SharedFlow<Unit> = _onClosed.asSharedFlow()

    private var session: WebSocketSession? = null
    private var lastPongTime: Long = System.currentTimeMillis()

    suspend fun connect(
        serverUrl: String,
        roomCode: String,
        playerName: String,
        playerId: String? = null,
        reconnectToken: String? = null
    ) {
        val url = socketUrl(serverUrl, roomCode, playerName, playerId, reconnectToken)
        val s = client.webSocketSession(url)
        session = s
        lastPongTime = System.currentTimeMillis()

        // Launch heartbeat on a separate coroutine
        scope.launch { pingLoop(s) }

        try {
            for (frame in s.incoming) {
                if (frame is Frame.Text) {
                    val envelope = json.decodeFromString<Envelope>(frame.readText())
                    if (envelope.type == "pong") {
                        lastPongTime = System.currentTimeMillis()
                    }
                    _incoming.emit(envelope)
                }
            }
        } finally {
            // Always close this attempt's socket, even on failure/cancellation.
            // Otherwise a retry after a client-perceived failure (e.g. during a
            // slow cold start) can leave the old, already-joined session open
            // on the server as a duplicate "ghost" player.
            withContext(NonCancellable) { runCatching { s.close() } }
            if (session === s) session = null
            _onClosed.emit(Unit)
        }
    }

    private suspend fun pingLoop(s: WebSocketSession) {
        while (true) {
            delay(20_000L)
            val pingEnv = Envelope(
                type = "ping",
                ts = System.currentTimeMillis(),
                data = buildJsonObject {}
            )
            try {
                s.send(Frame.Text(json.encodeToString(pingEnv)))
            } catch (e: Exception) {
                return
            }
            // Allow 10s for pong
            val timeout = withTimeoutOrNull(10_000L) {
                val deadline = System.currentTimeMillis() + 10_000L
                while (System.currentTimeMillis() - lastPongTime > 15_000L) {
                    if (System.currentTimeMillis() > deadline) return@withTimeoutOrNull false
                    delay(200)
                }
                true
            }
            if (timeout != true) {
                s.close()
                return
            }
        }
    }

    suspend fun send(envelope: Envelope) {
        session?.send(Frame.Text(json.encodeToString(envelope)))
    }

    suspend fun close() {
        session?.close()
        session = null
    }
}

/**
 * The upgrade URL for joining [roomCode]. A reconnect adds the player id and
 * the secret token from its welcome — the server refuses an id on its own,
 * since every player can see every other player's id.
 */
internal fun socketUrl(
    serverUrl: String,
    roomCode: String,
    playerName: String,
    playerId: String? = null,
    reconnectToken: String? = null
): String {
    val base = "$serverUrl?room=$roomCode&name=$playerName"
    return if (playerId != null) "$base&playerId=$playerId&token=${reconnectToken.orEmpty()}" else base
}
