package com.blindmap.android.ui

import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.widget.Toast
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.unit.dp
import com.blindmap.state.UpdateInfo

/**
 * Shown when the server reports a newer Android build than this one. A
 * required update (this build is below the server's minimum) is colored as an
 * error, since the player cannot actually play until they upgrade.
 *
 * The banner never installs directly — that would need the
 * REQUEST_INSTALL_PACKAGES permission, which Play treats as sensitive. It
 * offers two ways out instead: copy the link (for when handing off to the
 * browser fails or hangs, e.g. no default browser or a stalled Chrome), or
 * open it directly. The link itself is shown as selectable text so it can
 * always be read and copied by hand.
 */
@Composable
fun UpdateBanner(info: UpdateInfo, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val clipboard = LocalClipboardManager.current
    val required = info.required
    val link = apkLinkFrom(info.downloadUrl)

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
            Text(
                text = "Copy link rồi dán vào trình duyệt để tải file .apk:",
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.padding(top = 8.dp)
            )
            SelectionContainer {
                Text(
                    text = link,
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFF0D47A1),
                    modifier = Modifier.padding(top = 4.dp)
                )
            }
            Row(
                modifier = Modifier.padding(top = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Button(onClick = {
                    clipboard.setText(AnnotatedString(link))
                    // Android 13+ shows its own copy confirmation; a second
                    // toast on top of it just stacks up.
                    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) {
                        Toast.makeText(context, "Đã copy link", Toast.LENGTH_SHORT).show()
                    }
                }) {
                    Text("Copy link")
                }
                OutlinedButton(onClick = {
                    try {
                        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(link)))
                    } catch (e: ActivityNotFoundException) {
                        Toast.makeText(
                            context,
                            "Không mở được trình duyệt — hãy copy link và dán thủ công.",
                            Toast.LENGTH_LONG
                        ).show()
                    }
                }) {
                    Text("Mở trình duyệt")
                }
            }
        }
    }
}

/**
 * Turns the release *page* the server reports into a direct link to the APK
 * asset, so pasting it into a browser starts the download instead of landing
 * on a GitHub page that then has to be scrolled and tapped through.
 *
 * The asset name is fixed by .github/workflows/release.yml
 * (`blindmap-<versionName>.apk`, tag `v<versionName>`). Anything that does not
 * look like that release-tag URL is handed back untouched.
 */
internal fun apkLinkFrom(downloadUrl: String): String {
    val marker = "/releases/tag/v"
    val at = downloadUrl.indexOf(marker)
    if (at < 0) return downloadUrl
    val tag = downloadUrl.substring(at + marker.length - 1).trim('/')
    if (tag.length < 2 || tag.contains('/')) return downloadUrl
    val version = tag.substring(1)
    return downloadUrl.substring(0, at) + "/releases/download/$tag/blindmap-$version.apk"
}
