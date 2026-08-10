package com.blindmap.android.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeContentPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.blindmap.protocol.Position
import com.blindmap.state.ChatMessage
import com.blindmap.state.ConnState
import com.blindmap.state.GamePhase
import com.blindmap.state.MapMark
import com.blindmap.viewmodel.GameViewModel
import kotlinx.coroutines.delay
import kotlin.math.max

private val DIRECTIONS = listOf(
    "Lên" to "N", "Xuống" to "S", "Trái" to "W", "Phải" to "E"
)

@Composable
fun GameScreen(vm: GameViewModel, onNavigateToGameOver: () -> Unit) {
    val state by vm.uiState.collectAsState()
    val game = state.game

    var showShootDialog by remember { mutableStateOf(false) }
    var showMapDialog by remember { mutableStateOf(false) }
    var showInfoDialog by remember { mutableStateOf(false) }

    var chatPopup by remember { mutableStateOf<com.blindmap.state.ChatMessage?>(null) }
    val chatNotif = state.chatNotification
    LaunchedEffect(chatNotif?.ts) {
        if (chatNotif != null) {
            chatPopup = chatNotif
            delay(3000)
            chatPopup = null
            vm.dismissChatNotification()
        }
    }

    LaunchedEffect(state.transientError) {
        if (state.transientError != null) {
            delay(3000)
            vm.dismissTransientError()
        }
    }

    LaunchedEffect(state.phase) {
        if (state.phase == GamePhase.Ended) onNavigateToGameOver()
    }

    if (showShootDialog) {
        DirectionDialog(
            title = "Bắn về hướng nào?",
            onPick = { vm.sendShoot(it); showShootDialog = false },
            onDismiss = { showShootDialog = false }
        )
    }
    if (showMapDialog && game != null) {
        val portalPairs = (game.mapStats?.counts?.get("portal") ?: 0) / 2
        MapReconstructionDialog(
            mapSize = game.mapStats?.mapSize ?: 8,
            submitsLeft = game.self.submitsLeft,
            marks = state.mapMarks,
            portalPairs = portalPairs,
            onToggle = { pos -> vm.cycleMapMark(pos, portalPairs) },
            onSubmit = {
                val walls = state.mapMarks.filterValues { it == MapMark.Wall }.keys.toList()
                vm.sendSubmitMap(walls)
                showMapDialog = false
            },
            onClearAll = { vm.clearMapMarks() },
            onDismiss = { showMapDialog = false }
        )
    }
    if (showInfoDialog && game?.mapStats != null) {
        InfoDialog(
            mapSize = game.mapStats!!.mapSize,
            counts = game.mapStats!!.counts,
            onDismiss = { showInfoDialog = false }
        )
    }

    Box(modifier = Modifier.fillMaxSize().safeContentPadding().imePadding()) {
        Column(modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp, vertical = 8.dp)) {
            if (state.connState == ConnState.Reconnecting) {
                Banner("Đang kết nối lại…", MaterialTheme.colorScheme.errorContainer)
            } else if (state.connectionError != null) {
                Banner("Lỗi kết nối: ${state.connectionError}", MaterialTheme.colorScheme.errorContainer)
            }
            if (game?.paused == true) {
                Banner("⏸  ĐANG TẠM DỪNG — bấm Tiếp tục để chơi tiếp", MaterialTheme.colorScheme.tertiaryContainer)
            }

            game?.let { g ->
                val isMyTurn = g.currentTurn == g.self.id
                val secsLeft = max(0, ((g.turnEndsAt - System.currentTimeMillis()) / 1000).toInt())

                // Stable name→color mapping: sorted so every client (and every
                // recomposition) assigns the same color to the same player.
                val allNames = (listOf(g.self.name) + g.others.map { it.name }).distinct().sorted()
                val playerColors = remember(allNames) { playerColorMap(allNames) }

                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                    Text("Lượt ${g.turn}", style = MaterialTheme.typography.titleMedium)
                    Text(
                        if (isMyTurn) "Đến lượt bạn — ${secsLeft}s" else "Chờ — ${secsLeft}s",
                        style = MaterialTheme.typography.titleMedium,
                        color = if (isMyTurn) MaterialTheme.colorScheme.primary
                        else MaterialTheme.colorScheme.onBackground
                    )
                }

                Spacer(Modifier.height(4.dp))
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    val you = g.self
                    Text(
                        "${if (you.alive) "●" else "✗"} ${you.name}",
                        style = MaterialTheme.typography.bodySmall,
                        fontWeight = FontWeight.Bold,
                        color = playerColors[you.name] ?: MaterialTheme.colorScheme.onBackground
                    )
                    g.others.forEach { other ->
                        Text(
                            "${if (other.alive) "●" else "✗"} ${other.name}",
                            style = MaterialTheme.typography.bodySmall.copy(
                                textDecoration = if (other.alive) null else TextDecoration.LineThrough
                            ),
                            color = playerColors[other.name] ?: MaterialTheme.colorScheme.onBackground
                        )
                    }
                }
                if (g.self.inventory.any { it.kind == "bullet" }) {
                    Text("Bạn đang mang 1 viên đạn", style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.primary)
                }

                Spacer(Modifier.height(8.dp))

                Text("Diễn biến", style = MaterialTheme.typography.labelMedium)
                val listState = rememberLazyListState()
                // Follow the tail by default; a scroll gesture that ends away from
                // the bottom pauses following, one that ends near the bottom resumes it.
                var followTail by remember { mutableStateOf(true) }
                LaunchedEffect(listState) {
                    snapshotFlow { listState.isScrollInProgress }.collect { scrolling ->
                        if (!scrolling) {
                            val info = listState.layoutInfo
                            val last = info.visibleItemsInfo.lastOrNull()?.index ?: -1
                            followTail = info.totalItemsCount == 0 || last >= info.totalItemsCount - 2
                        }
                    }
                }
                LaunchedEffect(state.eventLog.size) {
                    if (state.eventLog.isNotEmpty() && followTail) {
                        listState.animateScrollToItem(state.eventLog.size - 1)
                    }
                }
                Card(modifier = Modifier.fillMaxWidth().weight(1f)) {
                    if (state.eventLog.isEmpty()) {
                        Text("Chưa có gì xảy ra.",
                            modifier = Modifier.padding(12.dp),
                            style = MaterialTheme.typography.bodySmall)
                    } else {
                        LazyColumn(state = listState, modifier = Modifier.padding(8.dp)) {
                            items(state.eventLog) { entry ->
                                EventLogLine(
                                    entry = entry,
                                    chipColor = entry.actor?.let { playerColors[it] },
                                    isSelf = entry.actor == g.self.name
                                )
                            }
                        }
                    }
                }

                Spacer(Modifier.height(8.dp))

                DPad(enabled = isMyTurn, onMove = { vm.sendMove(it) })

                Spacer(Modifier.height(8.dp))

                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                    val btnTextStyle = MaterialTheme.typography.labelSmall
                    val btnPadding = PaddingValues(horizontal = 4.dp, vertical = 8.dp)
                    if (g.self.inventory.any { it.kind == "bullet" }) {
                        OutlinedButton(
                            onClick = { showShootDialog = true },
                            enabled = isMyTurn,
                            modifier = Modifier.weight(1f),
                            contentPadding = btnPadding
                        ) { Text("Bắn", style = btnTextStyle, maxLines = 1) }
                    }
                    OutlinedButton(
                        onClick = { if (g.paused) vm.sendResume() else vm.sendPause() },
                        modifier = Modifier.weight(1f),
                        contentPadding = btnPadding
                    ) { Text(if (g.paused) "Tiếp tục" else "Tạm dừng", style = btnTextStyle, maxLines = 1) }
                    OutlinedButton(
                        onClick = { showMapDialog = true },
                        modifier = Modifier.weight(1f),
                        contentPadding = btnPadding
                    ) { Text("Bản đồ", style = btnTextStyle, maxLines = 1) }
                    OutlinedButton(
                        onClick = { showInfoDialog = true },
                        modifier = Modifier.weight(1f),
                        contentPadding = btnPadding
                    ) { Text("Thông tin", style = btnTextStyle, maxLines = 1) }
                }

                Spacer(Modifier.height(8.dp))
                ChatPanel(messages = state.chatMessages, onSend = { vm.sendChat(it) })
            }
        }

        chatPopup?.let { msg ->
            Card(
                modifier = Modifier
                    .align(Alignment.TopCenter)
                    .padding(top = 64.dp)
                    .fillMaxWidth(0.9f)
            ) {
                Text(
                    "💬 ${msg.senderName}: ${msg.text}",
                    modifier = Modifier.padding(12.dp),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.primary
                )
            }
        }

        state.transientError?.let { msg ->
            Card(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .padding(16.dp)
                    .fillMaxWidth()
            ) {
                Text(msg, modifier = Modifier.padding(12.dp),
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}

/**
 * One event feed line. Player-caused events get a rounded name chip filled with
 * the player's color (white text) followed by the description in normal text;
 * anonymous/system events render as a plain bullet line.
 */
@Composable
private fun EventLogLine(entry: com.blindmap.state.GameLogEntry, chipColor: Color?, isSelf: Boolean) {
    val actor = entry.actor
    if (actor == null || chipColor == null) {
        Text(
            "• ${entry.text}",
            style = MaterialTheme.typography.bodySmall,
            modifier = Modifier.padding(vertical = 2.dp)
        )
        return
    }
    // The narrator prefixes most lines with "Name — "; the chip replaces that.
    val prefix = "$actor — "
    val body = if (entry.text.startsWith(prefix)) entry.text.removePrefix(prefix) else entry.text
    Row(
        modifier = Modifier.padding(vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            actor,
            style = MaterialTheme.typography.labelSmall,
            fontWeight = FontWeight.Bold,
            color = Color.White,
            maxLines = 1,
            modifier = Modifier
                .background(chipColor, RoundedCornerShape(6.dp))
                .padding(horizontal = 6.dp, vertical = 1.dp)
        )
        Spacer(Modifier.width(6.dp))
        Text(
            body,
            style = MaterialTheme.typography.bodySmall,
            fontWeight = if (isSelf) FontWeight.Bold else null,
            modifier = Modifier.weight(1f)
        )
    }
}

@Composable
private fun Banner(text: String, bg: Color) {
    Card(modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp)) {
        Box(modifier = Modifier.fillMaxWidth().background(bg)) {
            Text(text, modifier = Modifier.padding(8.dp), style = MaterialTheme.typography.bodySmall)
        }
    }
}

@Composable
private fun DPad(enabled: Boolean, onMove: (String) -> Unit) {
    Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.fillMaxWidth()) {
        Button(onClick = { onMove("N") }, enabled = enabled, modifier = Modifier.width(96.dp)) { Text("↑") }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = { onMove("W") }, enabled = enabled, modifier = Modifier.width(96.dp)) { Text("←") }
            Button(onClick = { onMove("E") }, enabled = enabled, modifier = Modifier.width(96.dp)) { Text("→") }
        }
        Button(onClick = { onMove("S") }, enabled = enabled, modifier = Modifier.width(96.dp)) { Text("↓") }
    }
}

@Composable
private fun DirectionDialog(title: String, onPick: (String) -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                DIRECTIONS.forEach { (label, wire) ->
                    Button(onClick = { onPick(wire) }, modifier = Modifier.fillMaxWidth()) { Text(label) }
                }
            }
        },
        confirmButton = {},
        dismissButton = { TextButton(onClick = onDismiss) { Text("Hủy") } }
    )
}

/** Label + display color for each cycle step, applied on top of a white cell (wall is solid black). */
internal fun markLabel(mark: MapMark?): String = when (mark) {
    MapMark.Trap -> "T"
    MapMark.Bullet -> "B"
    is MapMark.Portal -> "P${mark.index}"
    else -> ""
}

/** Bigger cells on small maps (fewer columns to fit) so they stay easy to tap; shrinks back down as the map grows so it still fits the dialog. */
internal fun mapCellSize(mapSize: Int): androidx.compose.ui.unit.Dp = when {
    mapSize <= 8  -> 34.dp
    mapSize <= 12 -> 28.dp
    mapSize <= 16 -> 24.dp
    mapSize <= 20 -> 20.dp
    else          -> 16.dp
}

internal data class LegendItem(val label: String, val desc: String, val isWallSwatch: Boolean = false)
internal val MAP_LEGEND = listOf(
    LegendItem("", "Tường", isWallSwatch = true),
    LegendItem("T", "Bẫy"),
    LegendItem("B", "Đạn"),
    LegendItem("P1", "Cổng (P1, P2… theo từng cặp)"),
)

@Composable
internal fun MapLegend() {
    Row(
        modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
        horizontalArrangement = Arrangement.spacedBy(10.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        MAP_LEGEND.forEach { item ->
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(3.dp)) {
                Box(
                    modifier = Modifier
                        .size(14.dp)
                        .border(0.5.dp, Color.Gray)
                        .background(if (item.isWallSwatch) Color.Black else Color.White),
                    contentAlignment = Alignment.Center
                ) {
                    if (item.label.isNotEmpty()) {
                        Text(item.label, style = MaterialTheme.typography.labelSmall.copy(fontSize = 9.sp),
                            color = Color.Black, fontWeight = FontWeight.Bold)
                    }
                }
                Text(item.desc, style = MaterialTheme.typography.labelSmall)
            }
        }
    }
}

@Composable
private fun MapReconstructionDialog(
    mapSize: Int,
    submitsLeft: Int,
    marks: Map<Position, MapMark>,
    portalPairs: Int,
    onToggle: (Position) -> Unit,
    onSubmit: () -> Unit,
    onClearAll: () -> Unit,
    onDismiss: () -> Unit
) {
    var showClearConfirm by remember { mutableStateOf(false) }
    val cellSize = mapCellSize(mapSize)
    val portalHint = if (portalPairs > 0) "cổng (P1${if (portalPairs > 1) "..P$portalPairs" else ""})" else "cổng (bản đồ này không có cổng)"
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Vẽ lại bản đồ") },
        text = {
            Column {
                Text("Chạm vào ô để đổi: trống → tường (đen) → T (bẫy) → B (đạn) → $portalHint → trống. " +
                    "Ô thưởng không đánh dấu được vì nó liên tục đổi chỗ. Còn $submitsLeft lần nộp.",
                    style = MaterialTheme.typography.bodySmall)
                Text("Tọa độ trong Diễn biến là (cột,hàng) — khớp với nhãn số trên lưới. Chỉ ô tường được tính khi nộp.",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.6f))
                Spacer(Modifier.height(6.dp))
                Text("Chú thích:", style = MaterialTheme.typography.labelSmall, fontWeight = FontWeight.Bold)
                Spacer(Modifier.height(2.dp))
                MapLegend()
                Spacer(Modifier.height(14.dp))
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                    OutlinedButton(onClick = { showClearConfirm = true }) {
                        Text("Xóa trắng", style = MaterialTheme.typography.labelSmall)
                    }
                }
                Spacer(Modifier.height(14.dp))
                Column(modifier = Modifier.horizontalScroll(rememberScrollState())) {
                    val labelStyle = MaterialTheme.typography.labelSmall.copy(fontSize = 8.sp)
                    Row {
                        Box(Modifier.size(cellSize))
                        for (x in 1..mapSize) {
                            Box(Modifier.size(cellSize), contentAlignment = Alignment.Center) {
                                Text("$x", style = labelStyle)
                            }
                        }
                    }
                    for (y in 0 until mapSize) {
                        Row {
                            Box(Modifier.size(cellSize), contentAlignment = Alignment.Center) {
                                Text("${y + 1}", style = labelStyle)
                            }
                            for (x in 0 until mapSize) {
                                val pos = Position(x, y)
                                val mark = marks[pos]
                                Box(
                                    modifier = Modifier
                                        .size(cellSize)
                                        .border(0.5.dp, Color.Gray)
                                        .background(if (mark == MapMark.Wall) Color.Black else Color.White)
                                        .clickable { onToggle(pos) },
                                    contentAlignment = Alignment.Center
                                ) {
                                    val label = markLabel(mark)
                                    if (label.isNotEmpty()) {
                                        Text(label, style = labelStyle.copy(fontSize = (cellSize.value * 0.45f).sp), color = Color.Black,
                                            fontWeight = FontWeight.Bold)
                                    }
                                }
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {
            Button(onClick = onSubmit, enabled = submitsLeft > 0) { Text("Nộp") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Hủy") } }
    )
    if (showClearConfirm) {
        AlertDialog(
            onDismissRequest = { showClearConfirm = false },
            title = { Text("Xóa trắng bản đồ") },
            text = { Text("Có muốn xóa sạch bản đồ không?") },
            confirmButton = {
                Button(onClick = { onClearAll(); showClearConfirm = false }) { Text("Có") }
            },
            dismissButton = {
                TextButton(onClick = { showClearConfirm = false }) { Text("Không") }
            }
        )
    }
}

@Composable
internal fun ChatPanel(messages: List<ChatMessage>, onSend: (String) -> Unit) {
    var input by remember { mutableStateOf("") }
    val listState = rememberLazyListState()
    LaunchedEffect(messages.size) {
        if (messages.isNotEmpty()) listState.animateScrollToItem(messages.size - 1)
    }
    Column(modifier = Modifier.fillMaxWidth()) {
        Text("Trò chuyện", style = MaterialTheme.typography.labelMedium)
        Card(modifier = Modifier.fillMaxWidth().height(84.dp)) {
            if (messages.isEmpty()) {
                Text(
                    "Chưa có tin nhắn",
                    modifier = Modifier.padding(8.dp),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.5f)
                )
            } else {
                LazyColumn(state = listState, modifier = Modifier.padding(horizontal = 6.dp, vertical = 4.dp)) {
                    items(messages) { msg ->
                        Text(
                            "[${formatChatTs(msg.ts)}] ${msg.senderName}: ${msg.text}",
                            style = MaterialTheme.typography.bodySmall,
                            modifier = Modifier.padding(vertical = 1.dp)
                        )
                    }
                }
            }
        }
        Spacer(Modifier.height(4.dp))
        Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(4.dp),
            verticalAlignment = Alignment.CenterVertically) {
            OutlinedTextField(
                value = input,
                onValueChange = { if (it.length <= 200) input = it },
                placeholder = { Text("Nhắn tin…") },
                modifier = Modifier.weight(1f),
                singleLine = true,
                textStyle = MaterialTheme.typography.bodySmall
            )
            Button(
                onClick = { if (input.isNotBlank()) { onSend(input.trim()); input = "" } },
                enabled = input.isNotBlank()
            ) { Text("Gửi") }
        }
    }
}

internal fun formatChatTs(ts: Long): String {
    val cal = java.util.Calendar.getInstance()
    cal.timeInMillis = ts
    return "%02d:%02d".format(
        cal.get(java.util.Calendar.HOUR_OF_DAY),
        cal.get(java.util.Calendar.MINUTE)
    )
}

@Composable
private fun InfoDialog(mapSize: Int, counts: Map<String, Int>, onDismiss: () -> Unit) {
    // Server count keys → Vietnamese labels.
    val order = listOf(
        "wall" to "Tường",
        "blank" to "Ô trống",
        "trap" to "Bẫy",
        "reward" to "Phần thưởng",
        "bullet" to "Đạn",
        "portal" to "Cổng",
        "info" to "Ô thông tin",
    )
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Thông tin bản đồ") },
        text = {
            Column {
                Text("Bản đồ: ${mapSize}×$mapSize", fontWeight = FontWeight.Bold)
                Spacer(Modifier.height(8.dp))
                Row {
                    Text("Loại ô", modifier = Modifier.width(96.dp), fontWeight = FontWeight.Bold)
                    Text("Số lượng", fontWeight = FontWeight.Bold)
                }
                order.forEach { (key, label) ->
                    Row {
                        Text(label, modifier = Modifier.width(96.dp))
                        Text((counts[key] ?: 0).toString())
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text("Đóng") } }
    )
}
