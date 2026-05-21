package com.blindmap.android.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeContentPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.material3.Button
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
import androidx.compose.runtime.mutableFloatStateOf
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
import androidx.compose.foundation.layout.imePadding
import kotlinx.coroutines.delay

@Composable
fun LobbyScreen(vm: GameViewModel, serverUrl: String, onNavigateToGame: () -> Unit) {
    val state by vm.uiState.collectAsState()
    var playerName by remember { mutableStateOf("") }
    var roomCode by remember { mutableStateOf("") }
    var mapSize by remember { mutableIntStateOf(10) }
    var turnSeconds by remember { mutableIntStateOf(30) }

    LaunchedEffect(state.phase) {
        if (state.phase == GamePhase.Active) onNavigateToGame()
    }

    Column(
        modifier = Modifier.fillMaxSize().safeContentPadding().imePadding().padding(horizontal = 24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Text("Blind Map Survival", style = MaterialTheme.typography.headlineMedium)
        Spacer(Modifier.height(24.dp))

        OutlinedTextField(
            value = playerName,
            onValueChange = { playerName = it.take(20) },
            label = { Text("Your Name") },
            modifier = Modifier.fillMaxWidth()
        )
        Spacer(Modifier.height(12.dp))

        OutlinedTextField(
            value = roomCode,
            onValueChange = { roomCode = it.uppercase().take(6) },
            label = { Text("Room Code") },
            modifier = Modifier.fillMaxWidth()
        )
        Spacer(Modifier.height(16.dp))

        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(
                onClick = {
                    val code = generateRoomCode()
                    roomCode = code
                    vm.connect(serverUrl, code, playerName)
                },
                enabled = playerName.isNotBlank(),
                modifier = Modifier.weight(1f)
            ) { Text("Create Room") }

            OutlinedButton(
                onClick = { vm.connect(serverUrl, roomCode, playerName) },
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
                // Map settings
                Text("Map size: ${mapSize}×$mapSize", style = MaterialTheme.typography.bodySmall)
                Slider(
                    value = mapSize.toFloat(),
                    onValueChange = { mapSize = it.toInt() },
                    valueRange = 4f..20f,
                    steps = 15,
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

            Spacer(Modifier.height(12.dp))
            ChatPanel(messages = state.chatMessages, onSend = { vm.sendChat(it) })
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
    var progress by remember { mutableFloatStateOf(0f) }
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
