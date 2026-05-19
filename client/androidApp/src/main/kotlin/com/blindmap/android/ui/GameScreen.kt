package com.blindmap.android.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import com.blindmap.protocol.Position
import com.blindmap.state.ConnState
import com.blindmap.state.GamePhase
import com.blindmap.viewmodel.GameViewModel
import kotlinx.coroutines.delay
import kotlin.math.max

private val DIRECTIONS = listOf(
    "Up" to "N", "Down" to "S", "Left" to "W", "Right" to "E"
)

@Composable
fun GameScreen(vm: GameViewModel, onNavigateToGameOver: () -> Unit) {
    val state by vm.uiState.collectAsState()
    val game = state.game

    var showShootDialog by remember { mutableStateOf(false) }
    var showMapDialog by remember { mutableStateOf(false) }
    var showInfoDialog by remember { mutableStateOf(false) }

    var popup by remember { mutableStateOf<String?>(null) }
    var lastSeenSeq by remember { mutableStateOf(-1L) }
    val newest = state.eventLog.lastOrNull()
    LaunchedEffect(newest?.seq) {
        if (newest != null && newest.seq > lastSeenSeq) {
            lastSeenSeq = newest.seq
            popup = newest.text
            delay(3500)
            popup = null
        }
    }

    LaunchedEffect(state.transientError) {
        if (state.transientError != null) {
            delay(3000)
            vm.dismissTransientError()
        }
    }

    LaunchedEffect(state.phase) {
        if (state.phase == GamePhase.Ended) onNavigateToGameOver()
    }

    if (showShootDialog) {
        DirectionDialog(
            title = "Shoot which way?",
            onPick = { vm.sendShoot(it); showShootDialog = false },
            onDismiss = { showShootDialog = false }
        )
    }
    if (showMapDialog && game != null) {
        MapReconstructionDialog(
            mapSize = game.mapStats?.mapSize ?: 8,
            submitsLeft = game.self.submitsLeft,
            onSubmit = { walls -> vm.sendSubmitMap(walls); showMapDialog = false },
            onDismiss = { showMapDialog = false }
        )
    }
    if (showInfoDialog && game?.mapStats != null) {
        InfoDialog(
            mapSize = game.mapStats!!.mapSize,
            counts = game.mapStats!!.counts,
            onDismiss = { showInfoDialog = false }
        )
    }

    Box(modifier = Modifier.fillMaxSize()) {
        Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
            if (state.connState == ConnState.Reconnecting) {
                Banner("Reconnecting…", MaterialTheme.colorScheme.errorContainer)
            } else if (state.connectionError != null) {
                Banner("Connection problem: ${state.connectionError}", MaterialTheme.colorScheme.errorContainer)
            }

            game?.let { g ->
                val isMyTurn = g.currentTurn == g.self.id
                val secsLeft = max(0, ((g.turnEndsAt - System.currentTimeMillis()) / 1000).toInt())

                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                    Text("Turn ${g.turn}", style = MaterialTheme.typography.titleMedium)
                    Text(
                        if (isMyTurn) "Your turn — ${secsLeft}s" else "Waiting — ${secsLeft}s",
                        style = MaterialTheme.typography.titleMedium,
                        color = if (isMyTurn) MaterialTheme.colorScheme.primary
                        else MaterialTheme.colorScheme.onBackground
                    )
                }

                Spacer(Modifier.height(4.dp))
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    val you = g.self
                    Text(
                        "${if (you.alive) "●" else "✗"} ${you.name}",
                        style = MaterialTheme.typography.bodySmall,
                        fontWeight = FontWeight.Bold
                    )
                    g.others.forEach { other ->
                        Text(
                            "${if (other.alive) "●" else "✗"} ${other.name}",
                            style = MaterialTheme.typography.bodySmall.copy(
                                textDecoration = if (other.alive) null else TextDecoration.LineThrough
                            )
                        )
                    }
                }
                if (g.self.inventory.any { it.kind == "bullet" }) {
                    Text("You are carrying a bullet", style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.primary)
                }

                Spacer(Modifier.height(8.dp))

                Text("Events", style = MaterialTheme.typography.labelMedium)
                val listState = rememberLazyListState()
                LaunchedEffect(state.eventLog.size) {
                    if (state.eventLog.isNotEmpty()) {
                        listState.animateScrollToItem(state.eventLog.size - 1)
                    }
                }
                Card(modifier = Modifier.fillMaxWidth().weight(1f)) {
                    if (state.eventLog.isEmpty()) {
                        Text("Nothing has happened yet.",
                            modifier = Modifier.padding(12.dp),
                            style = MaterialTheme.typography.bodySmall)
                    } else {
                        LazyColumn(state = listState, modifier = Modifier.padding(8.dp)) {
                            items(state.eventLog) { entry ->
                                Text("• ${entry.text}",
                                    style = MaterialTheme.typography.bodySmall,
                                    modifier = Modifier.padding(vertical = 2.dp))
                            }
                        }
                    }
                }

                Spacer(Modifier.height(8.dp))

                DPad(enabled = isMyTurn, onMove = { vm.sendMove(it) })

                Spacer(Modifier.height(8.dp))

                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (g.self.inventory.any { it.kind == "bullet" }) {
                        OutlinedButton(
                            onClick = { showShootDialog = true },
                            enabled = isMyTurn,
                            modifier = Modifier.weight(1f)
                        ) { Text("Shoot") }
                    }
                    OutlinedButton(onClick = { showMapDialog = true }, modifier = Modifier.weight(1f)) { Text("Map") }
                    OutlinedButton(onClick = { showInfoDialog = true }, modifier = Modifier.weight(1f)) { Text("Info") }
                }
            }
        }

        popup?.let { msg ->
            Card(
                modifier = Modifier
                    .align(Alignment.TopCenter)
                    .padding(top = 64.dp)
                    .fillMaxWidth(0.9f)
            ) {
                Text(msg, modifier = Modifier.padding(12.dp), style = MaterialTheme.typography.bodyMedium)
            }
        }

        state.transientError?.let { msg ->
            Card(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .padding(16.dp)
                    .fillMaxWidth()
            ) {
                Text(msg, modifier = Modifier.padding(12.dp),
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}

@Composable
private fun Banner(text: String, bg: Color) {
    Card(modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp)) {
        Box(modifier = Modifier.fillMaxWidth().background(bg)) {
            Text(text, modifier = Modifier.padding(8.dp), style = MaterialTheme.typography.bodySmall)
        }
    }
}

@Composable
private fun DPad(enabled: Boolean, onMove: (String) -> Unit) {
    Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.fillMaxWidth()) {
        Button(onClick = { onMove("N") }, enabled = enabled, modifier = Modifier.width(96.dp)) { Text("↑") }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = { onMove("W") }, enabled = enabled, modifier = Modifier.width(96.dp)) { Text("←") }
            Button(onClick = { onMove("E") }, enabled = enabled, modifier = Modifier.width(96.dp)) { Text("→") }
        }
        Button(onClick = { onMove("S") }, enabled = enabled, modifier = Modifier.width(96.dp)) { Text("↓") }
    }
}

@Composable
private fun DirectionDialog(title: String, onPick: (String) -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                DIRECTIONS.forEach { (label, wire) ->
                    Button(onClick = { onPick(wire) }, modifier = Modifier.fillMaxWidth()) { Text(label) }
                }
            }
        },
        confirmButton = {},
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } }
    )
}

@Composable
private fun MapReconstructionDialog(
    mapSize: Int,
    submitsLeft: Int,
    onSubmit: (List<Position>) -> Unit,
    onDismiss: () -> Unit
) {
    var walls by remember { mutableStateOf(setOf<Pair<Int, Int>>()) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Reconstruct the map") },
        text = {
            Column {
                Text("Tap cells you think are walls. Attempts left: $submitsLeft",
                    style = MaterialTheme.typography.bodySmall)
                Spacer(Modifier.height(8.dp))
                Column(modifier = Modifier.horizontalScroll(rememberScrollState())) {
                    for (y in 0 until mapSize) {
                        Row {
                            for (x in 0 until mapSize) {
                                val key = x to y
                                val isWall = key in walls
                                Box(
                                    modifier = Modifier
                                        .size(20.dp)
                                        .border(0.5.dp, Color.Gray)
                                        .background(if (isWall) Color.Black else Color.White)
                                        .clickable { walls = if (isWall) walls - key else walls + key }
                                )
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {
            Button(
                onClick = { onSubmit(walls.map { Position(it.first, it.second) }) },
                enabled = submitsLeft > 0
            ) { Text("Submit") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } }
    )
}

@Composable
private fun InfoDialog(mapSize: Int, counts: Map<String, Int>, onDismiss: () -> Unit) {
    val order = listOf("wall", "blank", "trap", "reward", "bullet", "portal")
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Map info") },
        text = {
            Column {
                Text("Map: ${mapSize}x$mapSize", fontWeight = FontWeight.Bold)
                Spacer(Modifier.height(8.dp))
                Row {
                    Text("Type", modifier = Modifier.width(96.dp), fontWeight = FontWeight.Bold)
                    Text("Count", fontWeight = FontWeight.Bold)
                }
                order.forEach { type ->
                    Row {
                        Text(type, modifier = Modifier.width(96.dp))
                        Text((counts[type] ?: 0).toString())
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text("Close") } }
    )
}
