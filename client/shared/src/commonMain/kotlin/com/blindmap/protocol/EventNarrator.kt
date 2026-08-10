package com.blindmap.protocol

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/**
 * Name of the player who caused [event], or null for anonymous/system events.
 * Used by the UI to color each feed line by its actor.
 */
fun eventActorName(event: Event): String? {
    val p = event.payload as? JsonObject ?: return null
    fun str(key: String): String? = p[key]?.jsonPrimitive?.contentOrNull
    return when (event.kind) {
        "shot_fired", "player_eliminated", "you_were_eliminated" ->
            str("byPlayerName") ?: str("playerName")
        // Private intel about someone else (target's name may appear in the
        // payload) — not an action the reader's feed should attribute to them.
        "reward_activated", "info_revealed" -> null
        else -> str("playerName")
    }
}

fun describeEvent(event: Event): String {
    val p = event.payload as? JsonObject

    fun str(key: String): String? = p?.get(key)?.jsonPrimitive?.contentOrNull
    fun bool(key: String): Boolean? = p?.get(key)?.jsonPrimitive?.booleanOrNull
    fun int(key: String): Int? = p?.get(key)?.jsonPrimitive?.intOrNull
    // Wire positions are 0-based; players see 1-based (column,row) counted
    // from the top-left corner — matching the labels on the Map dialog grid.
    fun pos(key: String): String? {
        val obj = runCatching { p?.get(key)?.jsonObject }.getOrNull() ?: return null
        val x = obj["x"]?.jsonPrimitive?.intOrNull ?: return null
        val y = obj["y"]?.jsonPrimitive?.intOrNull ?: return null
        return "(${x + 1},${y + 1})"
    }

    return when (event.kind) {
        "player_moved" -> {
            val name = str("playerName") ?: "Ai đó"
            val dir = viDir(str("direction"))
            if (bool("success") != true) {
                "$name — đi $dir — đụng tường"
            } else when (str("blockType")) {
                "bullet" -> if (bool("bulletFull") == true)
                    "$name — đi $dir — gặp ô đạn (nhưng đang mang 1 viên rồi)"
                else "$name — đi $dir — nhặt được đạn"
                "reward" -> "$name — đi $dir — giẫm ô phần thưởng"
                "trap"   -> "$name — đi $dir — giẫm trúng bẫy"
                "portal" -> "$name — đi $dir — bước vào cổng dịch chuyển"
                else     -> "$name — đi $dir"
            }
        }

        "trap_triggered" -> when (str("effect")) {
            "reveal_position" -> {
                val who = str("playerName") ?: str("playerId") ?: "Một người"
                val at = pos("pos")
                if (at != null) "Bẫy — vị trí của $who tại $at"
                else "Bẫy — vị trí của $who bị lộ"
            }
            "random_teleport" -> {
                val who = str("playerName") ?: "Một người"
                "$who — bị dịch chuyển đến ô ngẫu nhiên"
            }
            "lose_next_turn" -> {
                val who = str("playerName") ?: "Một người"
                "Bẫy — $who bị mất lượt kế tiếp"
            }
            "lose_bullet" -> {
                val who = str("playerName") ?: "Một người"
                if (bool("lost") == true) "$who — bị mất viên đạn"
                else "$who — kích hoạt bẫy nhưng không có đạn để mất"
            }
            else -> "Bẫy được kích hoạt"
        }

        // Private — the server only sends this to the player who stepped on
        // the reward, so all five effects are personal intel, never public.
        "reward_activated" -> {
            val effectLine = when (str("effect")) {
                "own_position" -> {
                    val at = pos("pos")
                    if (at != null) "Phần thưởng — vị trí hiện tại của bạn là $at"
                    else "Phần thưởng — bạn được nhắc vị trí hiện tại của mình"
                }
                "own_start" -> {
                    val at = pos("pos")
                    if (at != null) "Phần thưởng — ô xuất phát của bạn là $at"
                    else "Phần thưởng — bạn được nhắc ô xuất phát của mình"
                }
                "other_position" -> {
                    val who = str("playerName")
                    val at = pos("pos")
                    if (who != null && at != null) "Phần thưởng — $who đang ở $at"
                    else "Phần thưởng — không còn người chơi nào khác còn sống"
                }
                "nearest_direction" -> {
                    val d = str("direction")
                    if (d == "none") "Phần thưởng — không còn người chơi nào khác"
                    else "Phần thưởng — người chơi gần nhất ở phía ${viSide(d)}"
                }
                "bullet_location" -> {
                    val at = pos("pos")
                    if (at != null) "Phần thưởng — có 1 ô đạn ở vị trí $at"
                    else "Phần thưởng — hiện không có ô đạn nào trên bản đồ"
                }
                else -> "Phần thưởng được kích hoạt"
            }
            val relocationLine = if (bool("relocated") == false) "Ô thưởng đã biến mất (bản đồ hết chỗ trống)."
            else "Ô thưởng đã di chuyển sang 1 ô ngẫu nhiên."
            "$effectLine\n$relocationLine"
        }

        "player_eliminated" -> {
            val victim = str("playerName") ?: "Một người"
            val by = str("byPlayerName")
            if (by != null) "$by — bắn trúng — $victim bị loại" else "$victim — bị loại"
        }

        "shot_fired" -> {
            val by = str("byPlayerName") ?: "Ai đó"
            val dir = str("direction") ?: ""
            if (dir.isNotEmpty()) "$by — bắn về phía ${viSide(dir)}" else "$by — đã nổ súng"
        }

        "map_submitted" -> {
            val name = str("playerName") ?: "Một người"
            if (bool("correct") == true) "$name — vẽ đúng bản đồ — THẮNG!"
            else {
                val wrong = int("wrong") ?: 0
                val left = int("submitsLeft")
                if (left != null) "$name — vẽ sai bản đồ (lệch $wrong ô, còn $left lần nộp)"
                else "$name — vẽ sai bản đồ (lệch $wrong ô)"
            }
        }

        "portal_used" -> {
            val name = str("playerName") ?: "Một người"
            "$name — dịch chuyển qua cổng"
        }

        // Private — the server only sends this to the player who stepped on the
        // info cell, so "bạn" wording is safe.
        "info_revealed" -> when (str("type")) {
            "surroundings_3x3" -> {
                val cells = runCatching { p?.get("cells")?.jsonArray }.getOrNull()
                val byRel = cells?.mapNotNull { el ->
                    val obj = runCatching { el.jsonObject }.getOrNull() ?: return@mapNotNull null
                    val rel = obj["rel"]?.jsonPrimitive?.contentOrNull ?: return@mapNotNull null
                    val kind = obj["kind"]?.jsonPrimitive?.contentOrNull ?: return@mapNotNull null
                    rel to kind
                }?.toMap() ?: emptyMap()
                val rows = (0..2).joinToString("\n") { r ->
                    (0..2).joinToString("") { c ->
                        if (r == 1 && c == 1) "🧍" else cellSymbol(byRel["$r-$c"])
                    }
                }
                "Thông tin — 8 ô xung quanh bạn (chỉ mình bạn thấy):\n$rows"
            }
            "player_position" -> {
                val who = str("playerName") ?: "Một người"
                val at = pos("pos")
                if (at != null) "Thông tin — $who đang ở $at (chỉ mình bạn thấy)"
                else "Thông tin — bạn được biết vị trí của một người chơi"
            }
            "own_start" -> {
                val at = pos("pos")
                if (at != null) "Thông tin — ô xuất phát của bạn là $at (chỉ mình bạn thấy)"
                else "Thông tin — bạn được biết ô xuất phát của mình"
            }
            else -> "Ô thông tin — bạn nhận được một gợi ý"
        }

        "you_were_eliminated" -> {
            val by = str("byPlayerName")
            if (by != null) "BẠN đã bị $by hạ gục" else "BẠN đã bị loại"
        }

        "turn_skipped" -> {
            val name = str("playerName") ?: "Một người"
            val suffix = when (str("reason")) {
                "timeout"     -> " (hết giờ)"
                "trap_effect" -> " (dính bẫy)"
                else          -> ""
            }
            "$name — bị mất lượt$suffix"
        }

        else -> event.kind.replace('_', ' ')
    }
}

// Wire directions are English ("up"/"down"/"left"/"right"); players see Vietnamese.
// viDir — movement phrasing ("đi lên"); viSide — bearing phrasing ("phía bên trái").
private fun viDir(dir: String?): String = when (dir) {
    "up"    -> "lên"
    "down"  -> "xuống"
    "left"  -> "sang trái"
    "right" -> "sang phải"
    else    -> "?"
}

private fun viSide(dir: String?): String = when (dir) {
    "up"    -> "trên"
    "down"  -> "dưới"
    "left"  -> "bên trái"
    "right" -> "bên phải"
    else    -> "?"
}

// Emoji legend for cell kinds — the 3×3 surroundings reveal (🧍 marks the
// player's own cell) and the full-map reveal on the game-over screen.
fun cellSymbol(kind: String?): String = when (kind) {
    "wall"                 -> "⬛"
    "empty"                -> "⬜"
    "bullet"               -> "🔶"
    "reward"               -> "🎁"
    "trap"                 -> "💀"
    "portal_a", "portal_b" -> "🌀"
    "info"                 -> "❓"
    else                   -> "⬜"
}
