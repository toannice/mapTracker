package com.blindmap.state

import com.blindmap.protocol.Envelope
import com.blindmap.protocol.Event
import com.blindmap.protocol.GameOverData
import com.blindmap.protocol.LobbyView
import com.blindmap.protocol.PlayerView
import com.blindmap.protocol.WelcomeData
import com.blindmap.protocol.describeEvent
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.jsonPrimitive

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
                connectionError = null
            )
        }
        "lobby_update" -> {
            val lobby = json.decodeFromJsonElement<LobbyView>(envelope.data)
            state.copy(lobby = lobby)
        }
        "game_start" -> {
            val view = json.decodeFromJsonElement<PlayerView>(envelope.data)
            // New match — reset the accumulating feed.
            val seeded = appendEvents(emptyList(), state.nextLogSeq, view.events, envelope.ts)
            state.copy(
                game = view,
                phase = GamePhase.Active,
                eventLog = seeded.first,
                nextLogSeq = seeded.second
            )
        }
        "turn_result" -> {
            val view = json.decodeFromJsonElement<PlayerView>(envelope.data)
            val appended = appendEvents(state.eventLog, state.nextLogSeq, view.events, envelope.ts)
            state.copy(
                game = view,
                eventLog = appended.first,
                nextLogSeq = appended.second
            )
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
            // Server action rejection — transient, the UI auto-dismisses it.
            val obj = envelope.data as? JsonObject
            val msg = obj?.get("message")?.jsonPrimitive?.contentOrNull
                ?: obj?.get("code")?.jsonPrimitive?.contentOrNull
                ?: envelope.data.toString()
            state.copy(transientError = msg)
        }
        "pong" -> state
        "server_shutdown" -> state.copy(connState = ConnState.Reconnecting)
        else -> state
    }
}

/**
 * Appends [events] to the accumulating log, formatting each into a narrative
 * sentence. Returns the new log and the next sequence id.
 */
private fun appendEvents(
    log: List<GameLogEntry>,
    startSeq: Long,
    events: List<Event>?,
    ts: Long
): Pair<List<GameLogEntry>, Long> {
    if (events.isNullOrEmpty()) return log to startSeq
    var seq = startSeq
    val additions = events.map { e ->
        GameLogEntry(seq = seq++, text = describeEvent(e), ts = ts)
    }
    return (log + additions) to seq
}
