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
    fun posStr(obj: JsonObject?): String? {
        val x = obj?.get("x")?.jsonPrimitive?.intOrNull
        val y = obj?.get("y")?.jsonPrimitive?.intOrNull
        return if (x != null && y != null) "($x,$y)" else null
    }

    return when (event.kind) {
        "player_moved" -> {
            val name = str("playerName") ?: "Someone"
            val dir = str("direction") ?: "?"
            if (bool("success") != true) {
                "$name moved $dir → hit a wall"
            } else when (str("blockType")) {
                "bullet"  -> if (bool("bulletFull") == true)
                    "$name moved $dir → bullet tile (already carrying one)"
                else "$name moved $dir → found a bullet"
                "reward"  -> "$name moved $dir → stepped on a reward"
                "trap"    -> "$name moved $dir → stepped on a trap"
                "info"    -> "$name moved $dir → stood on an info tile"
                "portal"  -> "$name moved $dir → stepped onto a portal"
                else      -> "$name moved $dir → blank tile"
            }
        }
        "trap_triggered" -> {
            val name = str("playerName") ?: "A player"
            when (str("effect")) {
                "reveal_position" -> {
                    val pos = runCatching { p?.get("pos")?.jsonObject }.getOrNull()
                    val coord = posStr(pos)
                    if (coord != null) "$name - trap → position revealed at $coord"
                    else "$name - trap → position revealed"
                }
                "random_teleport" -> "$name - trap → teleported to a random tile"
                "lose_next_turn"  -> "$name - trap → loses next turn"
                "lose_bullet"     -> "$name - trap → dropped their bullet"
                "info_blackout"   -> "$name - trap → info blacked out next turn"
                else -> "$name - trap triggered"
            }
        }
        "reward_activated" -> {
            val name = str("playerName") ?: "A player"
            when (str("effect")) {
                "all_positions_revealed" -> {
                    val arr = runCatching { p?.get("positions")?.jsonArray }.getOrNull()
                    if (arr != null) {
                        val parts = arr.mapNotNull { entry ->
                            val obj = runCatching { entry.jsonObject }.getOrNull() ?: return@mapNotNull null
                            val n = obj["name"]?.jsonPrimitive?.contentOrNull ?: return@mapNotNull null
                            val pos = runCatching { obj["pos"]?.jsonObject }.getOrNull()
                            val coord = posStr(pos)
                            if (coord != null) "$n at $coord" else n
                        }
                        "$name - reward → positions: ${parts.joinToString(", ")}"
                    } else {
                        "$name - reward → all positions revealed"
                    }
                }
                "nearest_direction" -> "$name - reward → nearest player is ${str("direction") ?: "?"}"
                "nuke_pending" -> {
                    val side = int("nukeSide") ?: 1
                    "$name triggered a nuke! Select a ${side}×${side} target area."
                }
                "all_bullet_locations" -> {
                    val arr = runCatching { p?.get("locations")?.jsonArray }.getOrNull()
                    if (arr != null) {
                        val parts = arr.mapNotNull { entry ->
                            val obj = runCatching { entry.jsonObject }.getOrNull() ?: return@mapNotNull null
                            posStr(obj)
                        }
                        "$name - reward → bullet tiles: ${parts.joinToString(", ")}"
                    } else {
                        "$name - reward → all bullet tiles revealed"
                    }
                }
                else -> "$name - reward activated"
            }
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
            if (bool("correct") == true) "$name submitted the map → correct! They win."
            else "$name submitted the map → ${int("wrong") ?: 0} cell(s) wrong"
        }
        "nuke_fired" -> {
            val by = str("byPlayerName") ?: "Someone"
            val topX = int("topX") ?: 0
            val topY = int("topY") ?: 0
            val side = int("side") ?: 1
            val eliminated = runCatching { p?.get("eliminated")?.jsonArray }.getOrNull()
            val hitList = eliminated?.mapNotNull { it.jsonPrimitive.contentOrNull }
            if (hitList.isNullOrEmpty()) {
                "$by nuked ${side}×${side} at ($topX,$topY) — no one hit"
            } else {
                "$by nuked ${side}×${side} at ($topX,$topY) — hit: ${hitList.joinToString(", ")}"
            }
        }
        "clue_received" -> {
            when (str("type")) {
                "nearest_direction" -> "Info: nearest player is ${str("direction") ?: "?"}"
                "own_start_pos" -> {
                    val pos = runCatching { p?.get("pos")?.jsonObject }.getOrNull()
                    val coord = posStr(pos)
                    if (coord != null) "Info: your start was $coord" else "Info: start position clue"
                }
                "other_player_pos" -> {
                    val other = str("playerName") ?: "A player"
                    val pos = runCatching { p?.get("pos")?.jsonObject }.getOrNull()
                    val coord = posStr(pos)
                    if (coord != null) "Info: $other is at $coord" else "Info: player location clue"
                }
                "surroundings_3x3" -> {
                    val arr = runCatching { p?.get("cells")?.jsonArray }.getOrNull()
                    if (arr != null) {
                        val parts = arr.mapNotNull { entry ->
                            val obj = runCatching { entry.jsonObject }.getOrNull() ?: return@mapNotNull null
                            val posObj = runCatching { obj["pos"]?.jsonObject }.getOrNull()
                            val coord = posStr(posObj)
                            val kind = obj["kind"]?.jsonPrimitive?.contentOrNull
                            if (coord != null && kind != null) "$coord=$kind" else null
                        }
                        "Info: surroundings ${parts.joinToString(" ")}"
                    } else {
                        "Info: surroundings clue"
                    }
                }
                else -> "You received a clue"
            }
        }
        "portal_used"   -> "A player was teleported through a portal"
        "turn_skipped"  -> "${str("playerName") ?: "A player"}'s turn was skipped"
        "blackout_info" -> "${str("playerName") ?: "A player"} have action but you are blackout info"
        else -> event.kind.replace('_', ' ')
    }
}
