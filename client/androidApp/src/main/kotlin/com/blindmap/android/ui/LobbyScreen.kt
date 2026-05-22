package com.blindmap.android.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeContentPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Slider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.blindmap.state.ConnState
import com.blindmap.state.GamePhase
import com.blindmap.viewmodel.GameViewModel
import kotlinx.coroutines.delay

data class SavedSession(
    val roomCode: String,
    val playerName: String,
    val playerId: String
)

@Composable
fun LobbyScreen(
    vm: GameViewModel,
    serverUrl: String,
    savedSession: SavedSession?,
    onSessionSave: (roomCode: String, playerName: String, playerId: String) -> Unit,
    onSessionClear: () -> Unit,
    onNavigateToGame: () -> Unit
) {
    val state by vm.uiState.collectAsState()
    var playerName by remember { mutableStateOf("") }
    var roomCode by remember { mutableStateOf("") }
    var mapSize by remember { mutableIntStateOf(20) }
    var turnSeconds by remember { mutableIntStateOf(30) }

    // Validation touched flags — only show errors after the user has interacted
    var nameTouched by remember { mutableStateOf(false) }
    var roomTouched by remember { mutableStateOf(false) }

    // Track whether this connect attempt is a rejoin (so we can detect rejoin failure)
    var isRejoining by remember { mutableStateOf(false) }
    // Track the player name used in the most recent connect call for session save
    var lastConnectName by remember { mutableStateOf("") }

    val nameError: String? = if (nameTouched && playerName.isBlank()) "Name is required" else null
    val roomError: String? = when {
        roomTouched && roomCode.isNotEmpty() && roomCode.length < 6 -> "Room code must be 6 characters"
        else -> null
    }

    LaunchedEffect(state.phase) {
        if (state.phase == GamePhase.Active) onNavigateToGame()
    }

    // Save session when playerId is received after a successful welcome
    LaunchedEffect(state.playerId) {
        if (state.playerId.isNotEmpty() && lastConnectName.isNotEmpty()) {
            val code = state.lobby?.roomCode ?: return@LaunchedEffect
            onSessionSave(code, lastConnectName, state.playerId)
        }
    }

    // Detect rejoin failure: if rejoining and a server error arrives, clear the saved session
    LaunchedEffect(state.transientError) {
        if (isRejoining && state.transientError != null) {
            onSessionClear()
            isRejoining = false
        }
    }

    Column(
        modifier = Modifier.fillMaxSize().safeContentPadding().imePadding().padding(horizontal = 24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Text("Blind Map Survival", style = MaterialTheme.typography.headlineMedium)
        Spacer(Modifier.height(24.dp))

        // ── Rejoin card ───────────────────────────────────────────────────────
        if (savedSession != null && state.playerId.isEmpty()) {
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.primaryContainer
                )
            ) {
                Column(modifier = Modifier.padding(16.dp)) {
                    Text(
                        "Last session: room ${savedSession.roomCode}",
                        style = MaterialTheme.typography.titleSmall
                    )
                    Text(
                        "as '${savedSession.playerName}'",
                        style = MaterialTheme.typography.bodySmall
                    )
                    Spacer(Modifier.height(8.dp))
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        Button(
                            onClick = {
                                isRejoining = true
                                lastConnectName = savedSession.playerName
                                vm.connect(
                                    serverUrl,
                                    savedSession.roomCode,
                                    savedSession.playerName,
                                    savedSession.playerId
                                )
                            },
                            modifier = Modifier.weight(1f)
                        ) { Text("Rejoin") }
                        OutlinedButton(
                            onClick = { onSessionClear() },
                            modifier = Modifier.weight(1f)
                        ) { Text("Dismiss") }
                    }
                }
            }
            Spacer(Modifier.height(16.dp))
            Text(
                "— or start a new session below —",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.5f)
            )
            Spacer(Modifier.height(12.dp))
        }

        // ── Name field ────────────────────────────────────────────────────────
        OutlinedTextField(
            value = playerName,
            onValueChange = {
                playerName = it.take(20)
                nameTouched = true
            },
            label = { Text("Your Name") },
            isError = nameError != null,
            supportingText = if (nameError != null) {
                { Text(nameError, color = MaterialTheme.colorScheme.error) }
            } else null,
            modifier = Modifier.fillMaxWidth()
        )
        Spacer(Modifier.height(12.dp))

        // ── Room code field ───────────────────────────────────────────────────
        OutlinedTextField(
            value = roomCode,
            onValueChange = {
                roomCode = it.uppercase().filter { c -> c.isLetterOrDigit() }.take(6)
                roomTouched = true
            },
            label = { Text("Room Code") },
            isError = roomError != null,
            supportingText = if (roomError != null) {
                { Text(roomError, color = MaterialTheme.colorScheme.error) }
            } else null,
            modifier = Modifier.fillMaxWidth()
        )
        Spacer(Modifier.height(16.dp))

        // ── Connect buttons ───────────────────────────────────────────────────
        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(
                onClick = {
                    if (playerName.isBlank()) { nameTouched = true; return@Button }
                    isRejoining = false
                    lastConnectName = playerName
                    val code = generateRoomCode()
                    roomCode = code
                    vm.connect(serverUrl, code, playerName)
                },
                enabled = playerName.isNotBlank(),
                modifier = Modifier.weight(1f)
            ) { Text("Create Room") }

            OutlinedButton(
                onClick = {
                    if (playerName.isBlank()) { nameTouched = true; return@OutlinedButton }
                    if (roomCode.length != 6) { roomTouched = true; return@OutlinedButton }
                    isRejoining = false
                    lastConnectName = playerName
                    vm.connect(serverUrl, roomCode, playerName)
                },
                enabled = playerName.isNotBlank() && roomCode.length == 6,
                modifier = Modifier.weight(1f)
            ) { Text("Join Room") }
        }

        Spacer(Modifier.height(24.dp))

        val players = state.lobby?.players ?: emptyList()
        if (players.isNotEmpty()) {
            Text("Players (${players.size}/8)", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(8.dp))
            LazyColumn(modifier = Modifier.fillMaxWidth()) {
                items(players) { name ->
                    Text("• $name", modifier = Modifier.padding(vertical = 2.dp))
                }
            }
            Spacer(Modifier.height(16.dp))

            val isHost = state.lobby?.isHost == true
            if (isHost) {
                Text("Map size: ${mapSize}×$mapSize", style = MaterialTheme.typography.bodySmall)
                Slider(
                    value = mapSize.toFloat(),
                    onValueChange = { mapSize = it.toInt() },
                    valueRange = 8f..40f,
                    steps = 7,
                    modifier = Modifier.fillMaxWidth()
                )
                Spacer(Modifier.height(4.dp))
                Text("Turn time: ${turnSeconds}s", style = MaterialTheme.typography.bodySmall)
                Slider(
                    value = turnSeconds.toFloat(),
                    onValueChange = { turnSeconds = it.toInt() },
                    valueRange = 10f..120f,
                    steps = 109,
                    modifier = Modifier.fillMaxWidth()
                )
                Spacer(Modifier.height(12.dp))
                Button(
                    onClick = { vm.sendStartGame(mapSize, turnSeconds) },
                    modifier = Modifier.fillMaxWidth()
                ) { Text(if (players.size == 1) "Start Solo" else "Start Game") }
            } else {
                Text("Waiting for host to start...", style = MaterialTheme.typography.bodySmall)
            }
        }

        if (state.connState == ConnState.Connecting || state.connState == ConnState.Reconnecting) {
            Spacer(Modifier.height(16.dp))
            ColdStartIndicator()
        }

        (state.connectionError ?: state.transientError)?.let { err ->
            Spacer(Modifier.height(8.dp))
            Text(err, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
        }
    }
}

@Composable
private fun ColdStartIndicator() {
    var elapsed by remember { mutableIntStateOf(0) }
    var progress by remember { mutableStateOf(0f) }
    val animatedProgress by animateFloatAsState(
        targetValue = progress,
        animationSpec = tween(durationMillis = 1000, easing = LinearEasing),
        label = "coldstart"
    )

    LaunchedEffect(Unit) {
        while (elapsed < 60) {
            delay(1000L)
            elapsed++
            progress = elapsed / 60f
        }
    }

    val message = when {
        elapsed < 5  -> "Connecting…"
        elapsed < 20 -> "Waking server… (free tier cold start, up to 60s)"
        elapsed < 45 -> "Still waking… ${60 - elapsed}s remaining"
        else         -> "Almost there… ${60 - elapsed}s"
    }

    Column(horizontalAlignment = Alignment.CenterHorizontally) {
        CircularProgressIndicator()
        Spacer(Modifier.height(8.dp))
        Text(message, style = MaterialTheme.typography.bodySmall)
        Spacer(Modifier.height(4.dp))
        LinearProgressIndicator(
            progress = { animatedProgress },
            modifier = Modifier.fillMaxWidth()
        )
        Text(
            "${elapsed}s / 60s",
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.5f)
        )
    }
}

private fun generateRoomCode(): String {
    val chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    return (1..6).map { chars.random() }.joinToString("")
}
