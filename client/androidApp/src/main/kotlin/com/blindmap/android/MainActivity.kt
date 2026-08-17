package com.blindmap.android

import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.graphics.Color
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import com.blindmap.android.ui.GameOverScreen
import com.blindmap.android.ui.GameScreen
import com.blindmap.android.ui.LobbyFormState
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
        enableEdgeToEdge()
        val serverUrl = getString(R.string.server_url)
        val versionCode = currentVersionCode()
        setContent {
            MaterialTheme(colorScheme = AppColorScheme) {
                Surface(color = MaterialTheme.colorScheme.background) {
                    BlindMapApp(serverUrl, versionCode)
                }
            }
        }
    }

    /** 0 when the platform will not tell us, which disables the update check. */
    private fun currentVersionCode(): Int = try {
        val info = packageManager.getPackageInfo(packageName, 0)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            info.longVersionCode.toInt()
        } else {
            @Suppress("DEPRECATION")
            info.versionCode
        }
    } catch (e: PackageManager.NameNotFoundException) {
        0
    }
}

@Composable
private fun BlindMapApp(serverUrl: String, versionCode: Int) {
    val navController = rememberNavController()
    val vm: GameViewModel = viewModel()
    vm.currentVersionCode = versionCode

    // Outlives any single visit to the lobby, so coming back from a finished
    // match still shows the room code and the match settings that were used.
    val lobbyForm = remember { LobbyFormState() }

    // Fire once per process, not per recomposition: the host sleeps when idle
    // and takes about a minute to wake, so the sooner it is poked the less of
    // that the player waits through after pressing Connect.
    LaunchedEffect(Unit) { vm.prewarmServer(serverUrl) }

    NavHost(navController = navController, startDestination = "lobby") {
        composable("lobby") {
            LobbyScreen(vm = vm, serverUrl = serverUrl, form = lobbyForm, onNavigateToGame = {
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
                // popUpTo("lobby") rather than ("gameover"): the lobby the
                // match started from is still on the stack, and without this
                // every match left another copy of it behind.
                navController.navigate("lobby") {
                    popUpTo("lobby") { inclusive = true }
                }
            })
        }
    }
}
