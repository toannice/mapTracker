package com.blindmap.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.blindmap.net.ReconnectManager
import com.blindmap.net.prewarm
import com.blindmap.net.WebSocketClient
import com.blindmap.protocol.ActionData
import com.blindmap.protocol.ChatData
import com.blindmap.protocol.Envelope
import com.blindmap.protocol.JoinData
import com.blindmap.protocol.Position
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

    /**
     * This build's Android versionCode, set once at startup. Left at 0 on
     * platforms that have none, which makes the update check a no-op.
     */
    var currentVersionCode: Int = 0

    private var prewarmed = false

    private var serverUrl: String = ""
    private var currentRoomCode: String = ""
    private var currentPlayerName: String = ""
    private var connectJob: kotlinx.coroutines.Job? = null

    /**
     * Starts waking the server before the player has finished typing. Runs at
     * most once per ViewModel — a second call would only re-pay the request
     * for a server that is already coming up.
     */
    fun prewarmServer(url: String) {
        if (prewarmed || url.isBlank()) return
        prewarmed = true
        viewModelScope.launch { prewarm(url) }
    }

    init {
        // Single collectors for the ViewModel's lifetime. Launching these inside
        // connect() registered a new SharedFlow subscriber per call, so every
        // envelope was reduced twice after a rejoin — duplicating event lines.
        viewModelScope.launch {
            wsClient.incoming.collect { envelope ->
                _uiState.value = reduce(_uiState.value, envelope, currentVersionCode)
            }
        }
        viewModelScope.launch {
            wsClient.onClosed.collect {
                val playerId = _uiState.value.playerId
                if (playerId.isNotEmpty()) {
                    val token = _uiState.value.reconnectToken
                    _uiState.value = _uiState.value.copy(connState = ConnState.Reconnecting)
                    reconnectManager.reconnect(playerId, token, serverUrl, currentRoomCode, currentPlayerName)
                    _uiState.value = _uiState.value.copy(connState = reconnectManager.connState.value)
                }
            }
        }
    }

    fun connect(serverUrl: String, roomCode: String, playerName: String) {
        this.serverUrl = serverUrl
        this.currentRoomCode = roomCode
        this.currentPlayerName = playerName

        connectJob?.cancel()
        connectJob = viewModelScope.launch {
            // Fresh room context: drop the old identity/lobby state and close any
            // previous socket, so the onClosed collector (which keys off playerId)
            // won't auto-reconnect to the room we are leaving.
            _uiState.value = ClientGameState(connState = ConnState.Connecting)
            wsClient.close()
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
                            connectionError = "Không kết nối được máy chủ sau 60 giây. Hãy thử lại."
                        )
                        break
                    }
                    attempt++
                    val backoff = minOf(1000L * (1L shl minOf(attempt - 1, 3)), 10_000L)
                    delay(minOf(backoff, remaining))
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

    fun sendStartGame(mapSize: Int = 20, turnSeconds: Int = 30) {
        sendAction(ActionData(kind = "start_game", mapSize = mapSize, turnSeconds = turnSeconds))
    }

    fun sendMove(direction: String) {
        sendAction(ActionData(kind = "move", direction = direction))
    }

    fun sendShoot(direction: String) {
        sendAction(ActionData(kind = "shoot", direction = direction))
    }

    /** Submits a reconstructed wall layout. Does not consume a turn. */
    fun sendSubmitMap(walls: List<Position>) {
        sendAction(ActionData(kind = "submit_map", walls = walls))
    }

    /** Cycles a map-reconstruction cell to its next mark (blank→wall→trap→bullet→portal 1..N→blank). */
    fun cycleMapMark(pos: Position, portalPairs: Int) {
        _uiState.value = com.blindmap.state.cycleMapMark(_uiState.value, pos, portalPairs)
    }

    /** Wipes every map-reconstruction mark back to blank ("Xóa trắng" action). */
    fun clearMapMarks() {
        _uiState.value = com.blindmap.state.clearMapMarks(_uiState.value)
    }

    fun sendPause() = sendAction(ActionData(kind = "pause"))
    fun sendResume() = sendAction(ActionData(kind = "resume"))

    /**
     * Post-game: rejoin this room's lobby on the current connection (no
     * reconnect) so the room can start a fresh match together with anyone
     * else who also returns. See [reset] for leaving the room entirely.
     */
    fun sendReturnToLobby() = sendAction(ActionData(kind = "return_to_lobby"))

    /** Lobby, host only: add a fair-mode bot ("easy" | "medium" | "hard"). */
    fun sendAddBot(difficulty: String) = sendAction(ActionData(kind = "add_bot", difficulty = difficulty))

    /** Lobby, host only: remove the most recently added bot. */
    fun sendRemoveBot() = sendAction(ActionData(kind = "remove_bot"))

    fun sendChat(text: String) {
        val trimmed = text.trim()
        if (trimmed.isEmpty() || trimmed.length > 200) return
        viewModelScope.launch {
            val envelope = Envelope(
                type = "chat",
                ts = Clock.System.now().toEpochMilliseconds(),
                data = json.encodeToJsonElement(ChatData(text = trimmed))
            )
            wsClient.send(envelope)
        }
    }

    /** Clears the transient error banner once the UI has shown it. */
    fun dismissTransientError() {
        _uiState.value = _uiState.value.copy(transientError = null)
    }

    fun dismissChatNotification() {
        _uiState.value = _uiState.value.copy(chatNotification = null)
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
