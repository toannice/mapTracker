package com.blindmap.state

import com.blindmap.protocol.GameOverData
import com.blindmap.protocol.LobbyView
import com.blindmap.protocol.PlayerView

// Idle = before user clicks Connect; Connecting = cold-start retry in flight
enum class ConnState { Idle, Connecting, Connected, Reconnecting, Failed }

enum class GamePhase { Lobby, Active, Ended }

/**
 * One narrative line in the accumulating event feed. [seq] is a monotonically
 * increasing id used by the UI to detect newly-arrived entries (for pop-ups).
 */
data class GameLogEntry(
    val seq: Long,
    val text: String,
    val ts: Long
)

data class ChatMessage(
    val senderName: String,
    val ts: Long,
    val text: String
)

data class ClientGameState(
    val roomId: String = "",
    val playerId: String = "",
    val phase: GamePhase = GamePhase.Lobby,
    val lobby: LobbyView? = null,
    val game: PlayerView? = null,
    val gameOver: GameOverData? = null,
    val connState: ConnState = ConnState.Idle,
    // Accumulated, human-readable event history — the client owns this since
    // the server only sends each turn's events once.
    val eventLog: List<GameLogEntry> = emptyList(),
    val nextLogSeq: Long = 0,
    // Server-side action rejections (NOT_YOUR_TURN, NO_BULLET, ...) — the UI
    // auto-dismisses these after a few seconds.
    val transientError: String? = null,
    // Network/reconnect failures — the UI shows these as a persistent banner.
    val connectionError: String? = null,
    // Chat messages for the current room, newest at the end (max 50).
    val chatMessages: List<ChatMessage> = emptyList()
)
