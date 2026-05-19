package com.blindmap.protocol

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/**
 * Turns a wire [Event] into a single human-readable sentence for the event
 * feed. Every action of every player is reported by real name — including the
 * local player (never "You" for moves/shots).
 */
fun describeEvent(event: Event): String {
    val p = event.payload as? JsonObject

    fun str(key: String): String? = p?.get(key)?.jsonPrimitive?.contentOrNull
    fun bool(key: String): Boolean? = p?.get(key)?.jsonPrimitive?.booleanOrNull
    fun int(key: String): Int? = p?.get(key)?.jsonPrimitive?.intOrNull

    return when (event.kind) {
        "player_moved" -> {
            val name = str("playerName") ?: "Someone"
            val dir = str("direction") ?: "?"
            if (bool("success") != true) {
                "$name moved $dir — hit a wall"
            } else when (str("blockType")) {
                "bullet" -> if (bool("bulletFull") == true)
                    "$name moved $dir — a bullet tile (already carrying one)"
                else "$name moved $dir — found a bullet"
                "reward" -> "$name moved $dir — stepped on a reward"
                "trap" -> "$name moved $dir — stepped on a trap"
                "portal" -> "$name moved $dir — stepped onto a portal"
                else -> "$name moved $dir — a blank tile"
            }
        }
        "trap_triggered" -> when (str("effect")) {
            "reveal_position" -> {
                val pos = p?.get("pos")?.jsonObject
                val x = pos?.get("x")?.jsonPrimitive?.intOrNull
                val y = pos?.get("y")?.jsonPrimitive?.intOrNull
                if (x != null && y != null) "A trap revealed a position at ($x, $y)"
                else "A trap revealed a player's position"
            }
            "random_teleport" -> "A trap teleported a player to a random tile"
            "lose_next_turn" -> "A trap froze a player — they lose their next turn"
            "lose_bullet" -> "A trap made a player drop their bullet"
            "info_blackout" -> "A trap blacked out a player's info"
            else -> "A trap was triggered"
        }
        "reward_activated" -> when (str("effect")) {
            "all_positions_revealed" -> "A reward revealed everyone's positions"
            "nearest_direction" -> "A reward pointed toward the nearest player (${str("direction") ?: "?"})"
            "all_bullet_locations" -> "A reward revealed all bullet tiles"
            else -> "A reward was activated"
        }
        "player_eliminated" -> {
            val victim = str("playerName") ?: "A player"
            val by = str("byPlayerName")
            if (by != null) "$by eliminated $victim" else "$victim was eliminated"
        }
        "shot_fired" -> {
            val by = str("byPlayerName") ?: "Someone"
            "$by fired a shot ${str("direction") ?: ""}".trim()
        }
        "map_submitted" -> {
            val name = str("playerName") ?: "A player"
            if (bool("correct") == true) "$name submitted the map — correct! They win."
            else "$name submitted the map — ${int("wrong") ?: 0} cell(s) wrong"
        }
        "portal_used" -> "A player was teleported through a portal"
        "clue_received" -> "You received a clue"
        "turn_skipped" -> "${str("playerName") ?: "A player"}'s turn was skipped"
        else -> event.kind.replace('_', ' ')
    }
}
