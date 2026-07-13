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
    val mapSize: Int? = null,
    val turnSeconds: Int? = null,
    val difficulty: String? = null,
    @SerialName("botId") val botId: String? = null
)

@Serializable
data class ChatData(val text: String)
