package com.blindmap.state

import com.blindmap.protocol.ChatHistoryData
import com.blindmap.protocol.ChatMsgData
import com.blindmap.protocol.Envelope
import com.blindmap.protocol.Event
import com.blindmap.protocol.GameOverData
import com.blindmap.protocol.LobbyView
import com.blindmap.protocol.PlayerView
import com.blindmap.protocol.WelcomeData
import com.blindmap.protocol.describeEvent
import com.blindmap.protocol.eventActorName
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
            // New match, new map — reset the accumulating feed and any
            // map-reconstruction notes left over from a previous match.
            val seeded = appendEvents(emptyList(), state.nextLogSeq, view.events, envelope.ts)
            state.copy(
                game = view,
                phase = GamePhase.Active,
                eventLog = seeded.first,
                nextLogSeq = seeded.second,
                mapMarks = emptyMap()
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
            // Known codes map to Vietnamese; unknown ones fall back to the
            // server's English message.
            val obj = envelope.data as? JsonObject
            val code = obj?.get("code")?.jsonPrimitive?.contentOrNull
            val msg = viError(code)
                ?: obj?.get("message")?.jsonPrimitive?.contentOrNull
                ?: code
                ?: envelope.data.toString()
            state.copy(transientError = msg)
        }
        "chat_msg" -> {
            val msg = json.decodeFromJsonElement<ChatMsgData>(envelope.data)
            val entry = ChatMessage(msg.senderName, msg.ts, msg.text)
            state.copy(
                chatMessages = (state.chatMessages + entry).takeLast(50),
                chatNotification = entry
            )
        }
        "chat_history" -> {
            val data = json.decodeFromJsonElement<ChatHistoryData>(envelope.data)
            val msgs = data.messages.map { ChatMessage(it.senderName, it.ts, it.text) }
            state.copy(chatMessages = msgs)
        }
        "pong" -> state
        "server_shutdown" -> state.copy(connState = ConnState.Reconnecting)
        else -> state
    }
}

/**
 * Advances [pos]'s mark to the next state in the cycle: blank → wall → trap
 * → bullet → portal 1 → portal 2 → … → portal [portalPairs] → blank. When
 * [portalPairs] is 0 the portal step is skipped entirely (nothing to mark).
 * Purely client-local (not a server action).
 */
fun cycleMapMark(state: ClientGameState, pos: com.blindmap.protocol.Position, portalPairs: Int): ClientGameState {
    val current = state.mapMarks[pos]
    val next: MapMark? = when (current) {
        null -> MapMark.Wall
        MapMark.Wall -> MapMark.Trap
        MapMark.Trap -> MapMark.Bullet
        MapMark.Bullet -> if (portalPairs > 0) MapMark.Portal(1) else null
        is MapMark.Portal -> {
            val nextIndex = current.index + 1
            if (nextIndex <= portalPairs) MapMark.Portal(nextIndex) else null
        }
    }
    val updated = if (next == null) state.mapMarks - pos else state.mapMarks + (pos to next)
    return state.copy(mapMarks = updated)
}

/** Wipes every map-reconstruction mark back to blank — the "Xóa trắng" action. */
fun clearMapMarks(state: ClientGameState): ClientGameState = state.copy(mapMarks = emptyMap())

private fun viError(code: String?): String? = when (code) {
    "NOT_YOUR_TURN"     -> "Chưa đến lượt của bạn"
    "NO_BULLET"         -> "Bạn không có đạn"
    "NO_SUBMIT_LEFT"    -> "Bạn đã hết lượt nộp bản đồ"
    "INVALID_DIRECTION" -> "Hướng đi không hợp lệ"
    "UNKNOWN_ACTION"    -> "Hành động không hợp lệ"
    "PLAYER_NOT_FOUND"  -> "Không tìm thấy người chơi"
    "ROOM_FULL"         -> "Phòng đã đầy (tối đa 8 người)"
    "GAME_IN_PROGRESS"  -> "Ván đấu đã bắt đầu, không vào được nữa"
    "BAD_DIFFICULTY"    -> "Độ khó bot không hợp lệ"
    else                -> null
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
        GameLogEntry(seq = seq++, text = describeEvent(e), ts = ts, actor = eventActorName(e))
    }
    return (log + additions) to seq
}
