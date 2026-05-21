package com.blindmap.protocol

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

fun describeEvent(event: Event): String {
    val p = event.payload as? JsonObject

    fun str(key: String): String? = p?.get(key)?.jsonPrimitive?.contentOrNull
    fun bool(key: String): Boolean? = p?.get(key)?.jsonPrimitive?.booleanOrNull
    fun int(key: String): Int? = p?.get(key)?.jsonPrimitive?.intOrNull
    fun pos(key: String): String? {
        val obj = runCatching { p?.get(key)?.jsonObject }.getOrNull() ?: return null
        val x = obj["x"]?.jsonPrimitive?.intOrNull ?: return null
        val y = obj["y"]?.jsonPrimitive?.intOrNull ?: return null
        return "($x,$y)"
    }

    return when (event.kind) {
        "player_moved" -> {
            val name = str("playerName") ?: "Someone"
            val dir = str("direction") ?: "?"
            if (bool("success") != true) {
                "$name — moved $dir — hit a wall"
            } else when (str("blockType")) {
                "bullet" -> if (bool("bulletFull") == true)
                    "$name — moved $dir — bullet tile (already carrying one)"
                else "$name — moved $dir — picked up a bullet"
                "reward" -> "$name — moved $dir — stepped on a reward"
                "trap"   -> "$name — moved $dir — triggered a trap"
                "portal" -> "$name — moved $dir — entered a portal"
                else     -> "$name — moved $dir"
            }
        }

        "trap_triggered" -> when (str("effect")) {
            "reveal_position" -> {
                val who = str("playerName") ?: str("playerId") ?: "A player"
                val at = pos("pos")
                if (at != null) "Trap — $who's position revealed at $at"
                else "Trap — $who's position was revealed"
            }
            "random_teleport" -> "Trap — a player was teleported to a random cell"
            "lose_next_turn"  -> "Trap — a player loses their next turn"
            "lose_bullet"     -> if (bool("lost") == true)
                "Trap — a player lost their bullet"
            else "Trap — triggered, but player had no bullet"
            "info_blackout"   -> "Trap — a player's map info is blacked out next turn"
            else -> "Trap triggered"
        }

        "reward_activated" -> when (str("effect")) {
            "all_positions_revealed" -> {
                val arr = runCatching { p?.get("positions")?.jsonArray }.getOrNull()
                if (!arr.isNullOrEmpty()) {
                    val list = arr.mapNotNull { el ->
                        val obj = runCatching { el.jsonObject }.getOrNull() ?: return@mapNotNull null
                        val name = obj["name"]?.jsonPrimitive?.contentOrNull ?: return@mapNotNull null
                        val posObj = runCatching { obj["pos"]?.jsonObject }.getOrNull() ?: return@mapNotNull null
                        val x = posObj["x"]?.jsonPrimitive?.intOrNull ?: return@mapNotNull null
                        val y = posObj["y"]?.jsonPrimitive?.intOrNull ?: return@mapNotNull null
                        "$name($x,$y)"
                    }.joinToString(" ")
                    "Reward — positions revealed: $list"
                } else "Reward — all positions revealed"
            }
            "nearest_direction" -> "Reward — nearest player is to the ${str("direction") ?: "?"}"
            "all_bullet_locations" -> {
                val locs = runCatching { p?.get("locations")?.jsonArray }.getOrNull()
                if (!locs.isNullOrEmpty()) {
                    val coords = locs.mapNotNull { el ->
                        val obj = runCatching { el.jsonObject }.getOrNull() ?: return@mapNotNull null
                        val x = obj["x"]?.jsonPrimitive?.intOrNull ?: return@mapNotNull null
                        val y = obj["y"]?.jsonPrimitive?.intOrNull ?: return@mapNotNull null
                        "($x,$y)"
                    }.joinToString(" ")
                    "Reward — bullet tiles at: $coords"
                } else "Reward — no bullet tiles on map"
            }
            else -> "Reward activated"
        }

        "player_eliminated" -> {
            val victim = str("playerName") ?: "A player"
            val by = str("byPlayerName")
            if (by != null) "$by — shot — $victim eliminated" else "$victim — eliminated"
        }

        "shot_fired" -> {
            val by = str("byPlayerName") ?: "Someone"
            val dir = str("direction") ?: ""
            if (dir.isNotEmpty()) "$by — fired $dir" else "$by — fired a shot"
        }

        "map_submitted" -> {
            val name = str("playerName") ?: "A player"
            if (bool("correct") == true) "$name — map correct — wins!"
            else {
                val wrong = int("wrong") ?: 0
                val left = int("submitsLeft")
                if (left != null) "$name — map wrong ($wrong cell(s) off, $left tries left)"
                else "$name — map wrong ($wrong cell(s) off)"
            }
        }

        "portal_used" -> {
            val dest = pos("dest")
            if (dest != null) "Portal — a player teleported to $dest"
            else "Portal — a player teleported"
        }

        "clue_received" -> when (str("type")) {
            "nearest_direction" -> "Clue — nearest player is to the ${str("direction") ?: "?"}"
            "own_start_pos"     -> {
                val at = pos("pos")
                if (at != null) "Clue — your starting position was $at" else "Clue — starting position hint"
            }
            "other_player_pos" -> {
                val name = str("playerName") ?: "another player"
                val at = pos("pos")
                if (at != null) "Clue — $name is at $at" else "Clue — another player's position revealed"
            }
            "surroundings_3x3" -> {
                val cells = runCatching { p?.get("cells")?.jsonArray }.getOrNull()
                if (!cells.isNullOrEmpty()) {
                    val parts = cells.mapNotNull { el ->
                        val obj = runCatching { el.jsonObject }.getOrNull() ?: return@mapNotNull null
                        val kind = obj["kind"]?.jsonPrimitive?.contentOrNull ?: return@mapNotNull null
                        val posObj = runCatching { obj["pos"]?.jsonObject }.getOrNull() ?: return@mapNotNull null
                        val x = posObj["x"]?.jsonPrimitive?.intOrNull ?: return@mapNotNull null
                        val y = posObj["y"]?.jsonPrimitive?.intOrNull ?: return@mapNotNull null
                        if (kind == "blank" || kind == "empty") null else "$kind($x,$y)"
                    }.filterNotNull()
                    if (parts.isEmpty()) "Clue — surroundings: all clear"
                    else "Clue — nearby: ${parts.joinToString(" ")}"
                } else "Clue — surroundings hint received"
            }
            else -> "Clue received"
        }

        "turn_skipped" -> "${str("playerName") ?: "A player"} — turn skipped"

        else -> event.kind.replace('_', ' ')
    }
}
