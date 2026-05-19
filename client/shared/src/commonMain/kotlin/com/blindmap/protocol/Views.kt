package com.blindmap.protocol

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

@Serializable
data class Position(val x: Int, val y: Int)

@Serializable
data class Item(val id: String, val kind: String)

@Serializable
data class SelfView(
    val id: String,
    val name: String,
    val pos: Position,
    val alive: Boolean,
    val inventory: List<Item>,
    @SerialName("visitedCount") val visitedCount: Int,
    @SerialName("totalCells") val totalCells: Int,
    @SerialName("infoBlackout") val infoBlackout: Boolean
)

@Serializable
data class OtherPlayerView(
    val id: String,
    val name: String,
    val alive: Boolean
)

@Serializable
data class CellView(val pos: Position, val kind: String)

@Serializable
data class Event(
    val kind: String,
    val payload: JsonElement? = null
)

fun Event.description(): String {
    val p = payload?.jsonObject
    return when (kind) {
        "trap_triggered"    -> "You triggered a trap!"
        "reward_activated"  -> "You found a reward!"
        "player_eliminated" -> {
            val name = p?.get("playerName")?.jsonPrimitive?.content
            val by   = p?.get("byPlayerName")?.jsonPrimitive?.content
            when {
                name != null && by != null -> "$name was eliminated by $by"
                name != null               -> "$name was eliminated!"
                else                       -> "A player was eliminated!"
            }
        }
        "shot_fired" -> {
            val dir = p?.get("direction")?.jsonPrimitive?.content
            val label = when (dir) { "N" -> "North"; "S" -> "South"; "E" -> "East"; "W" -> "West"; else -> dir }
            if (label != null) "Shot fired $label!" else "A shot was fired!"
        }
        "map_submitted" -> {
            val name = p?.get("playerName")?.jsonPrimitive?.content
            if (name != null) "$name submitted their map!" else "Map submitted!"
        }
        "portal_used"   -> "You teleported!"
        "clue_received" -> "Clue received!"
        "turn_skipped"  -> "Turn skipped (timed out)"
        else            -> kind
    }
}

@Serializable
data class PlayerView(
    val self: SelfView,
    val others: List<OtherPlayerView>,
    @SerialName("visibleMap") val visibleMap: List<CellView>,
    val events: List<Event>? = null,
    @SerialName("turnEndsAt") val turnEndsAt: Long,
    @SerialName("currentTurn") val currentTurn: String,
    val turn: Int,
    val phase: String
)

@Serializable
data class LobbyView(
    @SerialName("roomCode") val roomCode: String,
    val players: List<String>,
    @SerialName("isHost") val isHost: Boolean
)

@Serializable
data class WelcomeData(
    @SerialName("playerId") val playerId: String,
    @SerialName("roomState") val roomState: LobbyView
)

@Serializable
data class GameOverData(
    val winner: String?,
    @SerialName("winReason") val winReason: String
)
