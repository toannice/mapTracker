package com.blindmap.state

import com.blindmap.protocol.GameOverData
import com.blindmap.protocol.LobbyView
import com.blindmap.protocol.PlayerView

enum class ConnState { Connecting, Connected, Reconnecting, Failed }

enum class GamePhase { Lobby, Active, Ended }

data class ClientGameState(
    val roomId: String = "",
    val playerId: String = "",
    val phase: GamePhase = GamePhase.Lobby,
    val lobby: LobbyView? = null,
    val game: PlayerView? = null,
    val gameOver: GameOverData? = null,
    val connState: ConnState = ConnState.Connecting,
    val errorMessage: String? = null
)
