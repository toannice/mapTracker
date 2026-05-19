package com.blindmap.android.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Snackbar
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.TextButton
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import kotlinx.coroutines.delay
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import com.blindmap.protocol.ActionData
import com.blindmap.protocol.description
import com.blindmap.state.ConnState
import com.blindmap.state.GamePhase
import com.blindmap.viewmodel.GameViewModel
import kotlin.math.max

@Composable
fun GameScreen(vm: GameViewModel, onNavigateToGameOver: () -> Unit) {
    val state by vm.uiState.collectAsState()
    val game = state.game
    var snackbarMsg by remember { mutableStateOf<String?>(null) }
    var showShootDialog by remember { mutableStateOf(false) }

    if (showShootDialog) {
        AlertDialog(
            onDismissRequest = { showShootDialog = false },
            title = { Text("Shoot Direction") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    listOf("N", "S", "E", "W").forEach { dir ->
                        Button(
                            onClick = {
                                vm.sendAction(ActionData("shoot", direction = dir))
                                showShootDialog = false
                            },
                            modifier = Modifier.fillMaxWidth()
                        ) { Text(dir) }
                    }
                }
            },
            confirmButton = {},
            dismissButton = {
                TextButton(onClick = { showShootDialog = false }) { Text("Cancel") }
            }
        )
    }

    LaunchedEffect(state.phase) {
        if (state.phase == GamePhase.Ended) onNavigateToGameOver()
    }

    LaunchedEffect(state.errorMessage) {
        state.errorMessage?.let {
            snackbarMsg = it
            delay(3000L)
            snackbarMsg = null
        }
    }

    Column(
        modifier = Modifier.fillMaxSize().padding(16.dp),
        verticalArrangement = Arrangement.SpaceBetween
    ) {
        // Reconnect banner
        if (state.connState == ConnState.Reconnecting) {
            Card(modifier = Modifier.fillMaxWidth()) {
                Text("Reconnecting...", modifier = Modifier.padding(8.dp))
            }
        }

        // Turn info
        game?.let { g ->
            val isMyTurn = g.currentTurn == g.self.id
            val secsLeft = max(0, ((g.turnEndsAt - System.currentTimeMillis()) / 1000).toInt())

            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                Text("Turn ${g.turn}", style = MaterialTheme.typography.titleMedium)
                Text(if (isMyTurn) "Your turn! (${secsLeft}s)" else "Waiting... (${secsLeft}s)")
            }

            Spacer(Modifier.height(8.dp))

            // Map grid (visited cells)
            val gridSize = max(1, (g.visibleMap.maxOfOrNull { it.pos.x + 1 } ?: 1))
            val cellMap = g.visibleMap.associateBy { "${it.pos.x},${it.pos.y}" }
            val ownPos = g.self.pos
            val mapCols = gridSize.coerceAtLeast(5)

            Text("Map (${g.self.visitedCount}/${g.self.totalCells} explored)",
                style = MaterialTheme.typography.labelMedium)

            LazyVerticalGrid(
                columns = GridCells.Fixed(mapCols),
                modifier = Modifier.fillMaxWidth().height(200.dp)
            ) {
                items((0 until mapCols * mapCols).map { idx ->
                    val x = idx % mapCols
                    val y = idx / mapCols
                    val key = "$x,$y"
                    Pair(key, cellMap[key])
                }) { (key, cell) ->
                    val parts = key.split(",")
                    val x = parts[0].toInt()
                    val y = parts[1].toInt()
                    val isOwn = x == ownPos.x && y == ownPos.y
                    val bg = when {
                        isOwn -> Color(0xFF4CAF50)
                        cell != null -> cellColor(cell.kind)
                        else -> Color.LightGray.copy(alpha = 0.2f)
                    }
                    Box(
                        modifier = Modifier.size(28.dp).background(bg)
                            .border(0.5.dp, Color.Gray.copy(alpha = 0.3f))
                    )
                }
            }

            Spacer(Modifier.height(8.dp))

            // Exploration progress
            LinearProgressIndicator(
                progress = { g.self.visitedCount.toFloat() / g.self.totalCells.coerceAtLeast(1) },
                modifier = Modifier.fillMaxWidth()
            )

            Spacer(Modifier.height(8.dp))

            // Action buttons
            Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.fillMaxWidth()) {
                Row(horizontalArrangement = Arrangement.Center) {
                    Button(onClick = { vm.sendAction(ActionData("move", direction = "N")) },
                        enabled = isMyTurn) { Text("↑ N") }
                }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(onClick = { vm.sendAction(ActionData("move", direction = "W")) },
                        enabled = isMyTurn) { Text("← W") }
                    Button(onClick = { vm.sendAction(ActionData("move", direction = "S")) },
                        enabled = isMyTurn) { Text("↓ S") }
                    Button(onClick = { vm.sendAction(ActionData("move", direction = "E")) },
                        enabled = isMyTurn) { Text("→ E") }
                }
                Spacer(Modifier.height(4.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(onClick = { vm.sendAction(ActionData("pickup")) },
                        enabled = isMyTurn) { Text("Pick Up") }
                    if (g.self.inventory.any { it.kind == "bullet" }) {
                        OutlinedButton(onClick = { showShootDialog = true },
                            enabled = isMyTurn) { Text("Shoot") }
                    }
                    OutlinedButton(onClick = { vm.sendAction(ActionData("submit_map")) },
                        enabled = isMyTurn) { Text("Submit Map") }
                }
            }

            Spacer(Modifier.height(8.dp))

            // Opponents
            Text("Players", style = MaterialTheme.typography.labelMedium)
            g.others.forEach { other ->
                val style = if (!other.alive)
                    MaterialTheme.typography.bodySmall.copy(textDecoration = TextDecoration.LineThrough)
                else MaterialTheme.typography.bodySmall
                Text("${if (other.alive) "✓" else "✗"} ${other.name}", style = style)
            }

            Spacer(Modifier.height(8.dp))

            // Recent events
            val recentEvents = g.events.orEmpty()
            if (recentEvents.isNotEmpty()) {
                Text("Events", style = MaterialTheme.typography.labelMedium)
                LazyColumn(modifier = Modifier.height(80.dp)) {
                    items(recentEvents.takeLast(5)) { event ->
                        Text("• ${event.description()}", style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }

        // Snackbar for errors
        snackbarMsg?.let { msg ->
            Snackbar(
                action = { Button(onClick = { snackbarMsg = null }) { Text("OK") } }
            ) { Text(msg) }
        }
    }
}

private fun cellColor(kind: String): Color = when (kind) {
    "bullet" -> Color(0xFFFFEB3B)
    "reward" -> Color(0xFF8BC34A)
    "trap" -> Color(0xFFF44336)
    "portal_a", "portal_b" -> Color(0xFF9C27B0)
    else -> Color(0xFFBBDEFB)
}

