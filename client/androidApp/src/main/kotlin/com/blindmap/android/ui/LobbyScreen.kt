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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.TextButton
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
    // "create" | "join" — set khi user bấm nút lúc đang ở trong phòng khác,
    // để hỏi xác nhận trước khi rời phòng hiện tại.
    var confirmAction by remember { mutableStateOf<String?>(null) }

    val inRoom = state.lobby != null

    fun doCreate() {
        val code = generateRoomCode()
        roomCode = code
        vm.connect(serverUrl, code, playerName)
    }

    fun doJoin() = vm.connect(serverUrl, roomCode, playerName)

    LaunchedEffect(state.phase) {
        if (state.phase == GamePhase.Active) onNavigateToGame()
    }

    confirmAction?.let { action ->
        val currentRoom = state.lobby?.roomCode ?: ""
        AlertDialog(
            onDismissRequest = { confirmAction = null },
            title = { Text(if (action == "create") "Tạo phòng mới?" else "Vào phòng khác?") },
            text = {
                Text(
                    "Bạn đang ở trong phòng $currentRoom. " +
                        if (action == "create") "Có muốn rời đi và tạo phòng mới không?"
                        else "Có muốn rời đi và vào phòng $roomCode không?"
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    confirmAction = null
                    if (action == "create") doCreate() else doJoin()
                }) { Text("Có") }
            },
            dismissButton = {
                TextButton(onClick = { confirmAction = null }) { Text("Không") }
            }
        )
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .safeContentPadding()
            .imePadding()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 24.dp, vertical = 16.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Text("Blind Map Survival", style = MaterialTheme.typography.headlineMedium)
        Spacer(Modifier.height(24.dp))

        // Only ever set once the server has answered, so this cannot appear
        // before the player has successfully connected at least once.
        state.updateInfo?.let {
            UpdateBanner(it)
            Spacer(Modifier.height(16.dp))
        }

        OutlinedTextField(
            value = playerName,
            onValueChange = { playerName = it.take(20) },
            label = { Text("Tên của bạn") },
            modifier = Modifier.fillMaxWidth()
        )
        Spacer(Modifier.height(12.dp))

        OutlinedTextField(
            value = roomCode,
            onValueChange = { roomCode = it.uppercase().take(6) },
            label = { Text("Mã phòng") },
            modifier = Modifier.fillMaxWidth()
        )
        Spacer(Modifier.height(16.dp))

        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(
                onClick = { if (inRoom) confirmAction = "create" else doCreate() },
                enabled = playerName.isNotBlank(),
                modifier = Modifier.weight(1f)
            ) { Text("Tạo phòng") }

            OutlinedButton(
                onClick = { if (inRoom) confirmAction = "join" else doJoin() },
                enabled = playerName.isNotBlank() && roomCode.length == 6,
                modifier = Modifier.weight(1f)
            ) { Text("Vào phòng") }
        }

        Spacer(Modifier.height(24.dp))

        val players = state.lobby?.players ?: emptyList()
        if (players.isNotEmpty()) {
            Text("Người chơi (${players.size}/8)", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(8.dp))
            Column(modifier = Modifier.fillMaxWidth()) {
                players.forEach { name ->
                    Text("• $name", modifier = Modifier.padding(vertical = 2.dp))
                }
            }
            Spacer(Modifier.height(16.dp))

            val isHost = state.lobby?.isHost == true
            if (isHost) {
                // Map settings
                Text("Kích thước bản đồ: ${mapSize}×$mapSize", style = MaterialTheme.typography.bodySmall)
                Slider(
                    value = mapSize.toFloat(),
                    onValueChange = { mapSize = it.toInt() },
                    valueRange = 4f..20f,
                    steps = 15,
                    modifier = Modifier.fillMaxWidth()
                )
                Spacer(Modifier.height(4.dp))
                Text("Thời gian mỗi lượt: ${turnSeconds} giây", style = MaterialTheme.typography.bodySmall)
                Slider(
                    value = turnSeconds.toFloat(),
                    onValueChange = { turnSeconds = it.toInt() },
                    valueRange = 10f..120f,
                    steps = 109,
                    modifier = Modifier.fillMaxWidth()
                )
                Spacer(Modifier.height(8.dp))
                Text("Bot (người máy)", style = MaterialTheme.typography.bodySmall)
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(
                        onClick = { vm.sendAddBot("easy") },
                        modifier = Modifier.weight(1f)
                    ) { Text("+Dễ") }
                    OutlinedButton(
                        onClick = { vm.sendAddBot("medium") },
                        modifier = Modifier.weight(1f)
                    ) { Text("+Vừa") }
                    OutlinedButton(
                        onClick = { vm.sendAddBot("hard") },
                        modifier = Modifier.weight(1f)
                    ) { Text("+Khó") }
                    if (players.any { it.startsWith("Bot") }) {
                        OutlinedButton(
                            onClick = { vm.sendRemoveBot() },
                            modifier = Modifier.weight(0.6f)
                        ) { Text("−Bot") }
                    }
                }
                Spacer(Modifier.height(12.dp))
                Button(
                    onClick = { vm.sendStartGame(mapSize, turnSeconds) },
                    modifier = Modifier.fillMaxWidth()
                ) { Text(if (players.size == 1) "Chơi một mình" else "Bắt đầu") }
            } else {
                Text("Đang chờ chủ phòng bắt đầu…", style = MaterialTheme.typography.bodySmall)
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
        elapsed < 5  -> "Đang kết nối…"
        elapsed < 20 -> "Đang đánh thức máy chủ… (có thể mất tới 60 giây)"
        elapsed < 45 -> "Máy chủ vẫn đang khởi động… còn ${60 - elapsed} giây"
        else         -> "Sắp xong rồi… còn ${60 - elapsed} giây"
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
            "${elapsed} / 60 giây",
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.5f)
        )
    }
}

private fun generateRoomCode(): String {
    val chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    return (1..6).map { chars.random() }.joinToString("")
}
