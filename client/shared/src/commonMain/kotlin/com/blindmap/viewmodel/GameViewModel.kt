package com.blindmap.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.blindmap.net.ReconnectManager
import com.blindmap.net.WebSocketClient
import com.blindmap.protocol.ActionData
import com.blindmap.protocol.Envelope
import com.blindmap.protocol.JoinData
import com.blindmap.state.ClientGameState
import com.blindmap.state.ConnState
import com.blindmap.state.reduce
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.datetime.Clock
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.encodeToJsonElement

class GameViewModel : ViewModel() {
    private val json = Json { ignoreUnknownKeys = true }
    private val wsClient = WebSocketClient()
    private val reconnectManager = ReconnectManager(wsClient)

    private val _uiState = MutableStateFlow(ClientGameState())
    val uiState: StateFlow<ClientGameState> = _uiState

    private var serverUrl: String = ""
    private var currentRoomCode: String = ""
    private var currentPlayerName: String = ""

    fun connect(serverUrl: String, roomCode: String, playerName: String) {
        this.serverUrl = serverUrl
        this.currentRoomCode = roomCode
        this.currentPlayerName = playerName

        viewModelScope.launch {
            _uiState.value = _uiState.value.copy(connState = ConnState.Connecting, errorMessage = null)
            val coldStartDeadline = System.currentTimeMillis() + 60_000L
            var attempt = 0
            while (true) {
                try {
                    wsClient.connect(serverUrl, roomCode, playerName)
                    break  // session ended normally; onClosed collector handles reconnect
                } catch (e: Exception) {
                    val remaining = coldStartDeadline - System.currentTimeMillis()
                    if (remaining <= 0) {
                        _uiState.value = _uiState.value.copy(
                            connState = ConnState.Failed,
                            errorMessage = "Server unreachable after 60s. Try again."
                        )
                        break
                    }
                    attempt++
                    val backoff = minOf(1000L * (1L shl minOf(attempt - 1, 3)), 10_000L)
                    delay(minOf(backoff, remaining))
                }
            }
        }

        viewModelScope.launch {
            wsClient.incoming.collect { envelope ->
                _uiState.value = reduce(_uiState.value, envelope)
            }
        }

        viewModelScope.launch {
            wsClient.onClosed.collect {
                val playerId = _uiState.value.playerId
                if (playerId.isNotEmpty()) {
                    _uiState.value = _uiState.value.copy(connState = ConnState.Reconnecting)
                    reconnectManager.reconnect(playerId, serverUrl, roomCode, playerName)
                    _uiState.value = _uiState.value.copy(connState = reconnectManager.connState.value)
                }
            }
        }
    }

    fun sendAction(data: ActionData) {
        viewModelScope.launch {
            val envelope = Envelope(
                type = "action",
                ts = Clock.System.now().toEpochMilliseconds(),
                data = json.encodeToJsonElement(data)
            )
            wsClient.send(envelope)
        }
    }

    fun sendJoin(roomCode: String, playerName: String, playerId: String? = null) {
        viewModelScope.launch {
            val joinData = JoinData(
                roomCode = roomCode,
                playerName = playerName,
                playerId = playerId
            )
            val envelope = Envelope(
                type = "join",
                ts = Clock.System.now().toEpochMilliseconds(),
                data = json.encodeToJsonElement(joinData)
            )
            wsClient.send(envelope)
        }
    }

    fun sendStartGame() {
        sendAction(ActionData(kind = "start_game"))
    }

    fun reset() {
        viewModelScope.launch { wsClient.close() }
        _uiState.value = ClientGameState()
    }

    override fun onCleared() {
        super.onCleared()
        viewModelScope.launch { wsClient.close() }
    }
}
