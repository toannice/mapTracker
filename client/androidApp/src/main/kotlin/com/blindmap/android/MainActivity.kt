package com.blindmap.android

import android.content.Context
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import com.blindmap.android.ui.GameOverScreen
import com.blindmap.android.ui.GameScreen
import com.blindmap.android.ui.LobbyScreen
import com.blindmap.android.ui.SavedSession
import com.blindmap.viewmodel.GameViewModel

private val AppColorScheme = lightColorScheme(
    primary = Color(0xFF1565C0),
    onPrimary = Color.White,
    background = Color(0xFFF5F5F5),
    onBackground = Color(0xFF1A1A1A),
    surface = Color.White,
    onSurface = Color(0xFF1A1A1A),
)

private const val PREFS_NAME = "blindmap_session"
private const val KEY_ROOM   = "roomCode"
private const val KEY_NAME   = "playerName"
private const val KEY_ID     = "playerId"

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val serverUrl = getString(R.string.server_url)
        val prefs = getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
        setContent {
            MaterialTheme(colorScheme = AppColorScheme) {
                Surface(
                    color = MaterialTheme.colorScheme.background,
                    modifier = Modifier.safeDrawingPadding()
                ) {
                    BlindMapApp(
                        serverUrl = serverUrl,
                        loadSession = {
                            val roomCode   = prefs.getString(KEY_ROOM, null) ?: return@BlindMapApp null
                            val playerName = prefs.getString(KEY_NAME, null) ?: return@BlindMapApp null
                            val playerId   = prefs.getString(KEY_ID, null)   ?: return@BlindMapApp null
                            SavedSession(roomCode, playerName, playerId)
                        },
                        saveSession = { roomCode, playerName, playerId ->
                            prefs.edit()
                                .putString(KEY_ROOM, roomCode)
                                .putString(KEY_NAME, playerName)
                                .putString(KEY_ID, playerId)
                                .apply()
                        },
                        clearSession = {
                            prefs.edit().clear().apply()
                        }
                    )
                }
            }
        }
    }
}

@Composable
private fun BlindMapApp(
    serverUrl: String,
    loadSession: () -> SavedSession?,
    saveSession: (String, String, String) -> Unit,
    clearSession: () -> Unit
) {
    val navController = rememberNavController()
    val vm: GameViewModel = viewModel()
    var savedSession by remember { mutableStateOf(loadSession()) }

    NavHost(navController = navController, startDestination = "lobby") {
        composable("lobby") {
            LobbyScreen(
                vm = vm,
                serverUrl = serverUrl,
                savedSession = savedSession,
                onSessionSave = { roomCode, playerName, playerId ->
                    saveSession(roomCode, playerName, playerId)
                    savedSession = SavedSession(roomCode, playerName, playerId)
                },
                onSessionClear = {
                    clearSession()
                    savedSession = null
                },
                onNavigateToGame = {
                    navController.navigate("game")
                }
            )
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
