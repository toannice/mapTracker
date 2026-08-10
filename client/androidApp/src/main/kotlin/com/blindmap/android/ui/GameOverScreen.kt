package com.blindmap.android.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.blindmap.protocol.CellView
import com.blindmap.protocol.PlayerStartView
import com.blindmap.viewmodel.GameViewModel

@Composable
fun GameOverScreen(vm: GameViewModel, onPlayAgain: () -> Unit) {
    val state by vm.uiState.collectAsState()
    val gameOver = state.gameOver

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(32.dp),
        verticalArrangement = Arrangement.Top,
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Text("Kết thúc!", style = MaterialTheme.typography.headlineLarge, textAlign = TextAlign.Center)
        Spacer(Modifier.height(24.dp))

        val winner = gameOver?.winner
        if (winner != null) {
            Text("Người thắng: $winner", style = MaterialTheme.typography.headlineMedium, textAlign = TextAlign.Center)
            Spacer(Modifier.height(8.dp))
            val reason = when (gameOver.winReason) {
                "last_alive" -> "Người sống sót cuối cùng"
                "map_complete" -> "Vẽ đúng toàn bộ bản đồ trước tiên"
                else -> gameOver.winReason
            }
            Text(reason, style = MaterialTheme.typography.bodyLarge, textAlign = TextAlign.Center)
        } else {
            Text("Không có người thắng (hòa)", style = MaterialTheme.typography.headlineMedium, textAlign = TextAlign.Center)
        }

        if (gameOver != null && gameOver.mapSize > 0) {
            Spacer(Modifier.height(24.dp))
            Text("Bản đồ ban đầu — ai xuất phát ở đâu", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(8.dp))
            val playerColors = remember(gameOver.players) { playerColorMap(gameOver.players.map { it.name }) }
            if (gameOver.players.isNotEmpty()) {
                Row(modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    gameOver.players.forEach { pl ->
                        Text(
                            "${if (pl.alive) "●" else "✗"} ${pl.name}",
                            style = MaterialTheme.typography.bodySmall.copy(
                                textDecoration = if (pl.alive) null else TextDecoration.LineThrough
                            ),
                            color = playerColors[pl.name] ?: MaterialTheme.colorScheme.onBackground
                        )
                    }
                }
                Spacer(Modifier.height(8.dp))
            }
            Text("Chú thích:", style = MaterialTheme.typography.labelSmall, fontWeight = FontWeight.Bold)
            Spacer(Modifier.height(2.dp))
            MapLegend()
            Spacer(Modifier.height(8.dp))
            RevealedMapGrid(mapSize = gameOver.mapSize, cells = gameOver.map,
                players = gameOver.players, playerColors = playerColors)
        }

        Spacer(Modifier.height(32.dp))

        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            OutlinedButton(
                onClick = {
                    // Stay on this connection, in this room — the server keeps
                    // the room alive and moves it back to its lobby.
                    vm.sendReturnToLobby()
                    onPlayAgain()
                },
                modifier = Modifier.weight(1f)
            ) { Text("Quay về") }

            Button(
                onClick = {
                    vm.reset()
                    onPlayAgain()
                },
                modifier = Modifier.weight(1f)
            ) { Text("Thoát") }
        }
    }
}

@Composable
private fun RevealedMapGrid(
    mapSize: Int,
    cells: List<CellView>,
    players: List<PlayerStartView>,
    playerColors: Map<String, Color>
) {
    if (mapSize <= 0) return
    val kindByPos = remember(cells) { cells.associate { (it.pos.x to it.pos.y) to it.kind } }
    val startByPos = remember(players) { players.associateBy { it.startPos.x to it.startPos.y } }
    val cellSize = mapCellSize(mapSize)
    Column(modifier = Modifier.horizontalScroll(rememberScrollState())) {
        for (y in 0 until mapSize) {
            Row {
                for (x in 0 until mapSize) {
                    val starter = startByPos[x to y]
                    val kind = kindByPos[x to y]
                    val bg = when {
                        starter != null -> (playerColors[starter.name] ?: Color.Gray).copy(alpha = 0.45f)
                        kind == "wall" -> Color.Black
                        else -> Color.White
                    }
                    Box(
                        modifier = Modifier
                            .size(cellSize)
                            .border(0.5.dp, Color.Gray)
                            .background(bg),
                        contentAlignment = Alignment.Center
                    ) {
                        if (starter != null) {
                            Text(
                                starter.name.take(1).uppercase(),
                                fontSize = (cellSize.value * 0.6f).sp,
                                fontWeight = FontWeight.Bold,
                                color = playerColors[starter.name] ?: Color.Black
                            )
                        } else {
                            val label = mapLetterLabel(kind)
                            if (label.isNotEmpty()) {
                                Text(label, fontSize = (cellSize.value * 0.45f).sp, color = Color.Black,
                                    fontWeight = FontWeight.Bold)
                            }
                        }
                    }
                }
            }
        }
    }
}

/** Same letter scheme as the in-game map-reconstruction grid (wall = solid black, no emoji). */
private fun mapLetterLabel(kind: String?): String = when (kind) {
    "reward"               -> "R"
    "trap"                 -> "T"
    "bullet"               -> "B"
    "portal_a", "portal_b" -> "P"
    "info"                 -> "I"
    else                   -> ""
}
