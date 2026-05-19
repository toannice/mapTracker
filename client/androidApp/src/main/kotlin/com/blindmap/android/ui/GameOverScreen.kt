package com.blindmap.android.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.blindmap.viewmodel.GameViewModel

@Composable
fun GameOverScreen(vm: GameViewModel, onPlayAgain: () -> Unit) {
    val state by vm.uiState.collectAsState()
    val gameOver = state.gameOver

    Column(
        modifier = Modifier.fillMaxSize().padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Text("Game Over", style = MaterialTheme.typography.headlineLarge, textAlign = TextAlign.Center)
        Spacer(Modifier.height(24.dp))

        val winner = gameOver?.winner
        if (winner != null) {
            Text("Winner: $winner", style = MaterialTheme.typography.headlineMedium, textAlign = TextAlign.Center)
            Spacer(Modifier.height(8.dp))
            val reason = when (gameOver.winReason) {
                "last_alive" -> "Last player standing"
                "map_complete" -> "First to map the entire board"
                else -> gameOver.winReason
            }
            Text(reason, style = MaterialTheme.typography.bodyLarge, textAlign = TextAlign.Center)
        } else {
            Text("No winner (draw)", style = MaterialTheme.typography.headlineMedium, textAlign = TextAlign.Center)
        }

        Spacer(Modifier.height(32.dp))

        Button(
            onClick = {
                vm.reset()
                onPlayAgain()
            },
            modifier = Modifier.fillMaxWidth()
        ) {
            Text("Play Again")
        }
    }
}
