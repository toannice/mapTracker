package com.blindmap.android.ui

import androidx.compose.ui.graphics.Color

// 8 high-contrast colors on the light theme — one per player (max 8 per room).
// Assigned by sorted player name, so every client (and every screen) shows the
// same color for the same player.
val PLAYER_COLORS = listOf(
    Color(0xFFD32F2F), // red
    Color(0xFF1565C0), // blue
    Color(0xFF2E7D32), // green
    Color(0xFFEF6C00), // orange
    Color(0xFF6A1B9A), // purple
    Color(0xFF00838F), // teal
    Color(0xFFC2185B), // magenta
    Color(0xFF5D4037), // brown
)

/** Stable name→color mapping for a set of player names, sorted for determinism. */
fun playerColorMap(names: Collection<String>): Map<String, Color> {
    val sorted = names.distinct().sorted()
    return sorted.mapIndexed { i, n -> n to PLAYER_COLORS[i % PLAYER_COLORS.size] }.toMap()
}
