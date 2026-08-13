package com.blindmap.net

import io.ktor.client.HttpClient
import io.ktor.client.request.get
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.withTimeoutOrNull

// Waking a spun-down free-tier instance takes roughly a minute, and the
// request is held open for the whole wake-up rather than failing fast.
private const val PREWARM_TIMEOUT_MS = 90_000L

/**
 * Derives the health endpoint from the WebSocket URL the app is configured
 * with, so there is only ever one server address to keep correct.
 *
 * `wss://host/ws` -> `https://host/healthz`. Returns null for anything that is
 * not an http/ws URL, which simply skips the pre-warm.
 */
internal fun healthUrlFrom(serverUrl: String): String? {
    val trimmed = serverUrl.trim()
    val scheme = when {
        trimmed.startsWith("wss://", ignoreCase = true) -> "https://"
        trimmed.startsWith("ws://", ignoreCase = true) -> "http://"
        trimmed.startsWith("https://", ignoreCase = true) -> "https://"
        trimmed.startsWith("http://", ignoreCase = true) -> "http://"
        else -> return null
    }
    val authority = trimmed.substringAfter("://").substringBefore('/').substringBefore('?')
    if (authority.isEmpty()) return null
    return scheme + authority + "/healthz"
}

/**
 * Nudges the server awake as early as possible.
 *
 * The host spins down after idle, and the client used to trigger that wake-up
 * only when the player pressed Connect — leaving them watching a progress bar
 * for the full cold start. Calling this at app launch overlaps the wake-up
 * with the seconds spent typing a name and room code instead.
 *
 * Purely an optimisation: every failure is swallowed, because connect() does
 * its own retrying and reports problems to the player. This must never be the
 * thing that surfaces an error.
 */
suspend fun prewarm(serverUrl: String) {
    val url = healthUrlFrom(serverUrl) ?: return
    val client = HttpClient()
    try {
        withTimeoutOrNull(PREWARM_TIMEOUT_MS) { client.get(url) }
    } catch (e: CancellationException) {
        throw e
    } catch (e: Exception) {
        // Ignored on purpose — see kdoc.
    } finally {
        client.close()
    }
}
