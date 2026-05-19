package com.blindmap.android

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import com.blindmap.android.ui.GameOverScreen
import com.blindmap.android.ui.GameScreen
import com.blindmap.android.ui.LobbyScreen
import com.blindmap.viewmodel.GameViewModel

private val AppColorScheme = lightColorScheme(
    primary = Color(0xFF1565C0),
    onPrimary = Color.White,
    background = Color(0xFFF5F5F5),
    onBackground = Color(0xFF1A1A1A),
    surface = Color.White,
    onSurface = Color(0xFF1A1A1A),
)

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val serverUrl = getString(R.string.server_url)
        setContent {
            MaterialTheme(colorScheme = AppColorScheme) {
                Surface(color = MaterialTheme.colorScheme.background) {
                    BlindMapApp(serverUrl)
                }
            }
        }
    }
}

@Composable
private fun BlindMapApp(serverUrl: String) {
    val navController = rememberNavController()
    val vm: GameViewModel = viewModel()

    NavHost(navController = navController, startDestination = "lobby") {
        composable("lobby") {
            LobbyScreen(vm = vm, serverUrl = serverUrl, onNavigateToGame = {
                navController.navigate("game")
            })
        }
        composable("game") {
            GameScreen(vm = vm, onNavigateToGameOver = {
                navController.navigate("gameover") {
                    popUpTo("game") { inclusive = true }
                }
            })
        }
        composable("gameover") {
            GameOverScreen(vm = vm, onPlayAgain = {
                navController.navigate("lobby") {
                    popUpTo("gameover") { inclusive = true }
                }
            })
        }
    }
}
