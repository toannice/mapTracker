package com.blindmap.protocol

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

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
    @SerialName("infoBlackout") val infoBlackout: Boolean,
    @SerialName("submitsLeft") val submitsLeft: Int = 0
)

/** Aggregate-only map info backing the "Info" button — counts, never positions. */
@Serializable
data class MapStats(
    @SerialName("mapSize") val mapSize: Int,
    val counts: Map<String, Int>
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


@Serializable
data class PlayerView(
    val self: SelfView,
    val others: List<OtherPlayerView>,
    @SerialName("visibleMap") val visibleMap: List<CellView>,
    val events: List<Event>? = null,
    @SerialName("turnEndsAt") val turnEndsAt: Long,
    @SerialName("currentTurn") val currentTurn: String,
    val turn: Int,
    val phase: String,
    @SerialName("mapStats") val mapStats: MapStats? = null,
    val paused: Boolean = false
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

@Serializable
data class ChatMsgData(
    @SerialName("senderName") val senderName: String,
    val ts: Long,
    val text: String
)

@Serializable
data class ChatHistoryData(val messages: List<ChatMsgData>)
