package com.blindmap.state

import com.blindmap.protocol.GameOverData
import com.blindmap.protocol.LobbyView
import com.blindmap.protocol.PlayerView
import com.blindmap.protocol.Position

// Idle = before user clicks Connect; Connecting = cold-start retry in flight
enum class ConnState { Idle, Connecting, Connected, Reconnecting, Failed }

enum class GamePhase { Lobby, Active, Ended }

/**
 * A player's personal mark on the map-reconstruction grid — memory aid only,
 * never sent to the server except [Wall] cells (the only kind the server's
 * win check compares against). Reward is deliberately not markable: the
 * tile relocates every time anyone steps on it, so a fixed mark would go
 * stale immediately. [Portal.index] is the pair number (1-based, matching
 * how many portal pairs actually exist on this match's map) so a player can
 * tell two different portal pairs apart.
 */
sealed class MapMark {
    data object Wall : MapMark()
    data object Trap : MapMark()
    data object Bullet : MapMark()
    data class Portal(val index: Int) : MapMark()
}

/**
 * One narrative line in the accumulating event feed. [seq] is a monotonically
 * increasing id. [actor] is the name of the player who caused the event
 * (null for anonymous/system events) — the UI colors lines per player.
 */
data class GameLogEntry(
    val seq: Long,
    val text: String,
    val ts: Long,
    val actor: String? = null
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
    val chatMessages: List<ChatMessage> = emptyList(),
    // Transient: set to latest incoming chat message, cleared by UI after showing notification.
    val chatNotification: ChatMessage? = null,
    // Player's own map-reconstruction notes (wall/reward/trap/bullet/portal marks
    // on the redraw grid). Client-only, keyed by cell; reset each new match so
    // stale marks from a previous map never carry over.
    val mapMarks: Map<Position, MapMark> = emptyMap()
)
