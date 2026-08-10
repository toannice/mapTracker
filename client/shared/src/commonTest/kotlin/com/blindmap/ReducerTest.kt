package com.blindmap

import com.blindmap.protocol.Envelope
import com.blindmap.protocol.LobbyView
import com.blindmap.protocol.WelcomeData
import com.blindmap.state.ClientGameState
import com.blindmap.state.GamePhase
import com.blindmap.state.reduce
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.encodeToJsonElement
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull

class ReducerTest {
    private val json = Json { ignoreUnknownKeys = true }

    private fun envelopeOf(type: String, data: Any): Envelope {
        val elem = when (data) {
            is String -> json.parseToJsonElement(data)
            else -> json.encodeToJsonElement(data as WelcomeData)
        }
        return Envelope(type = type, ts = 0L, data = elem)
    }

    @Test
    fun welcomeSetsPlayerId() {
        val welcome = WelcomeData(
            playerId = "player-abc",
            roomState = LobbyView(roomCode = "ROOM01", players = listOf("Alice"), isHost = true)
        )
        val state = reduce(
            ClientGameState(),
            Envelope(type = "welcome", ts = 0L, data = json.encodeToJsonElement(welcome))
        )
        assertEquals("player-abc", state.playerId)
    }

    @Test
    fun lobbyUpdateUpdatesPlayers() {
        val lobbyJson = """{"roomCode":"ROOM01","players":["Alice","Bob"],"isHost":false}"""
        val state = reduce(
            ClientGameState(playerId = "alice"),
            Envelope(type = "lobby_update", ts = 0L, data = json.parseToJsonElement(lobbyJson))
        )
        assertEquals(2, state.lobby?.players?.size)
        assertEquals("Alice", state.lobby?.players?.get(0))
    }

    @Test
    fun gameStartSetsPhaseActive() {
        val gameStartJson = """{
            "self":{"id":"p1","name":"Alice","pos":{"x":0,"y":0},"alive":true,"inventory":[],"visitedCount":1,"totalCells":25,"infoBlackout":false},
            "others":[],
            "visibleMap":[{"pos":{"x":0,"y":0},"kind":"empty"}],
            "events":[],
            "turnEndsAt":9999999999999,
            "currentTurn":"p1",
            "turn":1,
            "phase":"active"
        }"""
        val state = reduce(
            ClientGameState(playerId = "p1"),
            Envelope(type = "game_start", ts = 0L, data = json.parseToJsonElement(gameStartJson))
        )
        assertEquals(GamePhase.Active, state.phase)
        assertNotNull(state.game)
    }

    @Test
    fun eventLogAccumulatesAcrossTurns() {
        fun turnResult(events: String) = """{
            "self":{"id":"p1","name":"Alice","pos":{"x":0,"y":0},"alive":true,"inventory":[],"visitedCount":1,"totalCells":25,"infoBlackout":false,"submitsLeft":3},
            "others":[],
            "visibleMap":[],
            "events":$events,
            "turnEndsAt":9999999999999,
            "currentTurn":"p1",
            "turn":1,
            "phase":"active"
        }"""

        var state = reduce(
            ClientGameState(playerId = "p1"),
            Envelope("game_start", 0L, json.parseToJsonElement(turnResult("[]")))
        )
        assertEquals(0, state.eventLog.size)

        state = reduce(
            state,
            Envelope("turn_result", 1L, json.parseToJsonElement(
                turnResult("""[{"kind":"player_moved","payload":{"playerName":"Alice","direction":"up","success":true,"blockType":"blank"}}]""")
            ))
        )
        assertEquals(1, state.eventLog.size)
        assertEquals("Alice — đi lên", state.eventLog[0].text)

        state = reduce(
            state,
            Envelope("turn_result", 2L, json.parseToJsonElement(
                turnResult("""[{"kind":"player_moved","payload":{"playerName":"Bob","direction":"left","success":false}}]""")
            ))
        )
        // Accumulates — the previous entry is still there.
        assertEquals(2, state.eventLog.size)
        assertEquals("Bob — đi sang trái — đụng tường", state.eventLog[1].text)
    }

    @Test
    fun errorEnvelopeSetsTransientError() {
        val state = reduce(
            ClientGameState(playerId = "p1"),
            Envelope("error", 0L, json.parseToJsonElement("""{"code":"NOT_YOUR_TURN","message":"it is not your turn"}"""))
        )
        // Known codes are localized to Vietnamese for the player.
        assertEquals("Chưa đến lượt của bạn", state.transientError)
    }

    @Test
    fun otherPlayerViewHasNoPos() {
        val gameStartJson = """{
            "self":{"id":"p1","name":"Alice","pos":{"x":0,"y":0},"alive":true,"inventory":[],"visitedCount":1,"totalCells":25,"infoBlackout":false},
            "others":[{"id":"p2","name":"Bob","alive":true}],
            "visibleMap":[],
            "events":[],
            "turnEndsAt":9999999999999,
            "currentTurn":"p1",
            "turn":1,
            "phase":"active"
        }"""
        val state = reduce(
            ClientGameState(playerId = "p1"),
            Envelope(type = "game_start", ts = 0L, data = json.parseToJsonElement(gameStartJson))
        )
        val bob = state.game?.others?.firstOrNull { it.name == "Bob" }
        assertNotNull(bob)
        assertEquals("Bob", bob.name)
        // OtherPlayerView only has id, name, alive — no pos field (compile-time guarantee via data class)
    }
}
