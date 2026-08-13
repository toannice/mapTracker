package com.blindmap.android.ui

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.blindmap.state.UpdateInfo

/**
 * Shown when the server reports a newer Android build than this one. A
 * required update (this build is below the server's minimum) is colored as an
 * error, since the player cannot actually play until they upgrade.
 *
 * Tapping opens the download page in a browser rather than installing
 * directly — that keeps the app free of the REQUEST_INSTALL_PACKAGES
 * permission, which Play treats as sensitive.
 */
@Composable
fun UpdateBanner(info: UpdateInfo, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val required = info.required

    Card(
        modifier = modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = if (required) Color(0xFFFFEBEE) else Color(0xFFE3F2FD)
        )
    ) {
        Column(modifier = Modifier.padding(12.dp)) {
            Text(
                text = if (required) "Bắt buộc cập nhật" else "Đã có bản mới",
                style = MaterialTheme.typography.titleSmall,
                color = if (required) Color(0xFFB71C1C) else Color(0xFF0D47A1)
            )
            Text(
                text = if (required) {
                    "Bản này quá cũ để chơi trên máy chủ hiện tại. " +
                        "Tải bản ${info.latestVersionCode} để tiếp tục."
                } else {
                    "Bản ${info.latestVersionCode} đã sẵn sàng."
                },
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.padding(top = 2.dp)
            )
            Button(
                onClick = {
                    context.startActivity(
                        Intent(Intent.ACTION_VIEW, Uri.parse(info.downloadUrl))
                    )
                },
                modifier = Modifier.padding(top = 8.dp)
            ) {
                Text("Tải bản mới")
            }
        }
    }
}
