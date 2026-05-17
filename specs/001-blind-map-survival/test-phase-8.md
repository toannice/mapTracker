# Test Guide: Phase 8 — User Story 6 (Reconnect After Disconnection)

**Phase goal**: A disconnected player's slot is preserved. They can rejoin with their original `playerId` and receive current game state. Client uses exponential backoff (1s×2^attempt, cap 30s, max 5 attempts).

---

## Prerequisites

- Phase 7 complete and passing
- Server running: `MAP_SIZE=5 TURN_SECONDS=15 LOG_LEVEL=debug go run ./cmd/server`
- Use `TURN_SECONDS=15` so turn-skip-while-disconnected is testable quickly

---

## Test 1: Player Slot Preserved on Disconnect

### Setup

Start 2-player game (Alice + Bob). Note Alice's `playerId` from `welcome`. Make 2 moves. Then disconnect Alice (Ctrl+C in wscat).

### Expected — server behavior

Server logs show:
```
level=INFO msg="player disconnected" roomId=GAME01 playerName=Alice
```
(NOT "player left" / "player removed" — slot must be preserved)

Bob's game continues normally. On Alice's next turn, the 15-second timer expires and fires:
```json
{
  "type": "turn_result",
  "data": {
    "events": [{"kind":"turn_skipped","payload":{"playerName":"Alice","reason":"timeout"}}],
    "currentTurn": "<Bob's ID>",
    ...
  }
}
```

### Wrong if

- Server logs "player removed" or "player deleted" → slot is being destroyed; must only unregister Conn, not Player
- Bob gets `lobby_update` with only Bob in players list → Player struct was removed from GameState
- Game ends immediately with Bob as winner → "last alive" check must not count disconnected players as eliminated

---

## Test 2: Reconnect Restores State

Alice reconnects to the same room within the retry window using her original `playerId`:

```bash
wscat -c "ws://localhost:8080/ws?room=GAME01&name=Alice"
```

Send join with original playerId:
```json
{
  "type": "join",
  "ts": 0,
  "data": {
    "roomCode": "GAME01",
    "playerName": "Alice",
    "clientVersion": "1.0.0",
    "playerId": "<alice's original playerId>"
  }
}
```

### Expected — Alice receives

```json
{
  "type": "welcome",
  "ts": <epoch_ms>,
  "data": {
    "playerId": "<same alice playerId as before>",
    "roomState": {
      "roomCode": "GAME01",
      "players": ["Alice", "Bob"],
      "isHost": true
    }
  }
}
```

Immediately followed by a `turn_result` with Alice's current game state (her position, inventory, visited cells — all preserved from before disconnection).

### Wrong if

- `welcome` returns a NEW `playerId` → server created new player instead of restoring existing slot; playerId lookup by incoming `playerId` field failed
- `turn_result` shows `visitedCount: 1` when Alice had visited 3 cells → visited cells not preserved during disconnect
- Alice's inventory is empty after reconnect when she had a bullet → inventory not preserved
- Bob's `others` list shows Alice as dead after reconnect → `Alive` field corrupted during disconnect handling

---

## Test 3: Turn Skip While Disconnected

### Setup

Disconnect Alice. Wait for 3 consecutive turn cycles (45 seconds with TURN_SECONDS=15) without reconnecting.

### Expected

Each time Alice's turn arrives, it auto-skips:
```json
{"kind":"turn_skipped","payload":{"playerName":"Alice","reason":"timeout"}}
```

Alice's turns skip 3 times. Bob takes 3 turns normally. Game does not end.

### Wrong if

- Game ends after 1 skip → `checkWinCondition` counting disconnected players as eliminated
- Turn gets stuck (no one's turn advances) → TurnDeadline not being reset after skip of disconnected player
- Bob receives `game_over{winner:"Bob"}` → last-alive check must distinguish disconnected (slot preserved) from eliminated (Alive=false)

---

## Test 4: Exponential Backoff (Client-Side)

Test the Kotlin `ReconnectManager` behavior. Add debug logging to `ReconnectManager` that prints the delay before each attempt. Simulate a server that is offline.

Disconnect Alice, stop the server, then have the Android client try to reconnect (or use unit test):

### Expected delays between reconnect attempts

| Attempt | Delay |
|---|---|
| 1 | 1 second |
| 2 | 2 seconds |
| 3 | 4 seconds |
| 4 | 8 seconds |
| 5 | 16 seconds |
| (cap) | 30 seconds max |

### How to verify (unit test in `shared/src/commonTest`)

```kotlin
class ReconnectManagerTest {
    @Test
    fun testBackoffDelays() {
        val delays = (0..5).map { attempt ->
            minOf(2.0.pow(attempt).toLong() * 1000L, 30_000L)
        }
        assertEquals(listOf(1000L, 2000L, 4000L, 8000L, 16000L, 30000L), delays)
    }
}
```

Run: `cd client && .\gradlew :shared:test`

### Wrong if

- All delays are 1 second → backoff exponent not applied; using fixed delay
- Delay exceeds 30 seconds → cap not applied; `minOf(delay, 30_000)` missing
- Attempt count resets → counter not persisted across retry loop iterations

---

## Test 5: ConnState.Failed After 5 Attempts

Stop the server. Have the client attempt reconnect and exhaust all 5 attempts.

### Expected (Android UI)

After 5 failed attempts, UI shows "Connection failed" full-screen state with a return-to-lobby button.

### Expected (ReconnectManager logs)

```
Reconnect attempt 1/5 failed
Reconnect attempt 2/5 failed
Reconnect attempt 3/5 failed
Reconnect attempt 4/5 failed
Reconnect attempt 5/5 failed
ConnState → Failed
```

### Wrong if

- Reconnect loops infinitely past 5 → attempt counter max check missing
- Failed state not emitted → `ConnState.Failed` not pushed to StateFlow after loop exits
- UI still shows "Reconnecting…" after failure → ConnState.Failed not observed in GameScreen

---

## Test 6: Ping/Pong Heartbeat

Start a game. Let it idle (no moves) for 25 seconds (past the 20-second ping interval).

### Expected — client sends

```json
{"type":"ping","ts":<epoch_ms>,"data":{}}
```

### Expected — server replies

```json
{"type":"pong","ts":<epoch_ms>,"data":{}}
```

Check wscat terminal — pong should appear ~20 seconds after game start.

### Wrong if

- No ping sent after 20 seconds → ping coroutine not launched or interval wrong
- No pong received after ping → server's pong handler not wired; check room.go for `type=="ping"` handling
- Connection drops without pong → 10-second timeout too short; verify it's set to 10_000ms

---

## Test 7: Server Shutdown — Client Reconnects

Send SIGTERM to the server (`Ctrl+C` in server terminal). Observe:

1. Server sends `server_shutdown` before closing:
```json
{"type":"server_shutdown","ts":0,"data":{"reconnectAfterMs":5000}}
```

2. Client (Android) shows "Server restarting…" and waits 5 seconds.
3. Server is restarted manually.
4. Client attempts reconnect after 5 seconds using ReconnectManager.

### Wrong if

- Server closes connections without sending `server_shutdown` → graceful shutdown not implemented; send `server_shutdown` to all conns before accepting SIGTERM context cancellation
- Client reconnects immediately (< 5 seconds) → `reconnectAfterMs` value not respected

---

## Test 8: All Players Disconnected — Draw

Start 2-player game. Disconnect BOTH Alice and Bob without either reconnecting.

### Expected

After a reasonable grace period (e.g., both turns skipped once each), server sends `game_over` to any remaining connection (there are none, but server should log it):

```json
{"type":"game_over","data":{"winner":null,"winReason":"last_alive"}}
```

`winner` is `null` when all players are disconnected (draw condition).

### Wrong if

- Server panics when all conns are nil → nil conn map access; guard with `len(conns) == 0` before broadcast
- `winner` is non-null when all disconnected → wrong player selected as winner

---

## Phase 8 Pass Criteria

| Test | Expected |
|---|---|
| Disconnect preserves slot | Player struct stays in GameState |
| Turn skip while disconnected | Auto-skip fires each cycle; game continues |
| Reconnect with same playerId | Same playerId, state fully restored |
| visitedCount restored | Same count as before disconnect |
| Backoff delays correct | 1s, 2s, 4s, 8s, 16s (cap 30s) |
| Failed after 5 attempts | ConnState.Failed, UI shows failure state |
| Ping sent every 20s | Client pings; server replies with pong |
| server_shutdown message | Sent before server closes; client waits reconnectAfterMs |
| All-disconnected game | game_over with winner=null |

**Proceed to Phase 9 only when all checks pass.**
