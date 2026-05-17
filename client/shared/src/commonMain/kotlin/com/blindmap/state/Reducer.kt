package com.blindmap.state

import com.blindmap.protocol.Envelope
import com.blindmap.protocol.GameOverData
import com.blindmap.protocol.LobbyView
import com.blindmap.protocol.PlayerView
import com.blindmap.protocol.WelcomeData
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.decodeFromJsonElement

private val json = Json { ignoreUnknownKeys = true }

fun reduce(state: ClientGameState, envelope: Envelope): ClientGameState {
    return when (envelope.type) {
        "welcome" -> {
            val data = json.decodeFromJsonElement<WelcomeData>(envelope.data)
            state.copy(
                playerId = data.playerId,
                lobby = data.roomState,
                phase = GamePhase.Lobby,
                connState = ConnState.Connected,
                errorMessage = null
            )
        }
        "lobby_update" -> {
            val lobby = json.decodeFromJsonElement<LobbyView>(envelope.data)
            state.copy(lobby = lobby)
        }
        "game_start" -> {
            val view = json.decodeFromJsonElement<PlayerView>(envelope.data)
            state.copy(game = view, phase = GamePhase.Active)
        }
        "turn_result" -> {
            val view = json.decodeFromJsonElement<PlayerView>(envelope.data)
            state.copy(game = view)
        }
        "event" -> {
            // events are delivered inside turn_result PlayerView.events; this handles standalone events
            state
        }
        "game_over" -> {
            val data = json.decodeFromJsonElement<GameOverData>(envelope.data)
            state.copy(gameOver = data, phase = GamePhase.Ended)
        }
        "error" -> {
            // ErrorData has code + message; surface the message
            val msg = envelope.data.toString()
            state.copy(errorMessage = msg)
        }
        "pong" -> state
        "server_shutdown" -> state.copy(connState = ConnState.Reconnecting)
        else -> state
    }
}
