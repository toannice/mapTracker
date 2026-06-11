package com.blindmap.protocol

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

@Serializable
data class Envelope(
    val type: String,
    val ts: Long,
    val data: JsonElement
)

@Serializable
data class JoinData(
    @SerialName("roomCode") val roomCode: String,
    @SerialName("playerName") val playerName: String,
    @SerialName("clientVersion") val clientVersion: String = "1.0",
    @SerialName("playerId") val playerId: String? = null
)

@Serializable
data class ActionData(
    val kind: String,
    val direction: String? = null,
    @SerialName("itemId") val itemId: String? = null,
    val walls: List<Position>? = null,
    @SerialName("mapSize") val mapSize: Int? = null,
    @SerialName("turnSeconds") val turnSeconds: Int? = null,
    @SerialName("nukeX") val nukeX: Int? = null,
    @SerialName("nukeY") val nukeY: Int? = null
)
