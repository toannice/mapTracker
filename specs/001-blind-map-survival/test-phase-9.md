# Test Guide: Phase 9 — Polish & Cross-Cutting Concerns

**Phase goal**: Input validation (rate limit, origin check), structured logging, Docker image < 20 MB, Render.com deployment passes health check, all go tests and Kotlin tests pass.

---

## Prerequisites

- Phases 1–8 complete and passing
- Docker installed and running
- Server source in `server/`

---

## T049 — go vet

### How to test

```powershell
cd server
go vet ./...
```

### Expected

No output, exit code 0.

### Common failures and fixes

| Error | Fix |
|---|---|
| `printf format %s has arg of wrong type` | Match format verbs to argument types |
| `unreachable code` | Remove dead code after return/panic |
| `composite literal uses unkeyed fields` | Use named fields: `Position{X: 1, Y: 2}` |
| `sync.Mutex must not be copied` | Pass by pointer, not value |

---

## T050 — go test (Race Detector)

### How to test

```powershell
cd server
go test -race -timeout 60s ./...
```

### Expected

```
ok   server/internal/config      0.001s
ok   server/internal/game        0.003s
ok   server/internal/protocol    0.003s
ok   server/internal/hub         0.010s
ok   server/internal/room        0.020s
```

All packages pass. No `DATA RACE` output.

### Data Race: What It Means

```
DATA RACE
Write at 0x... by goroutine N:
  server/internal/room.(*Room).Run(...)

Previous read at 0x... by goroutine M:
  server/internal/room.(*Room).handleJoin(...)
```

This means two goroutines are accessing the same memory without synchronization.

**Root cause in this project**: If any code outside the Room goroutine reads or writes `GameState` fields without going through the `inCh` channel, it's a data race.

**Fix rule**: All `GameState` mutations and reads must happen inside the Room goroutine's `Run()` select loop. If another goroutine needs state, it sends a message to `inCh` and waits for a reply channel.

### Test Coverage Expectations

Minimum tests expected in each package:

| Package | Tests |
|---|---|
| `internal/config` | Load defaults, Load overrides |
| `internal/game` | GenerateMap cell counts, VisitedCells dedup |
| `internal/protocol` | BuildPlayerView no position leak, BuildPlayerView visited-only map |
| `internal/room` | advanceTurn move valid, advanceTurn boundary rejected, NOT_YOUR_TURN |

---

## T051 — Kotlin Shared Tests

### How to test

```powershell
cd client
.\gradlew :shared:test
```

### Expected

```
BUILD SUCCESSFUL
> Task :shared:jvmTest
> Task :shared:testDebugUnitTest

X tests completed, 0 failed
```

### Minimum tests expected

| Test | Validates |
|---|---|
| `ReducerTest.welcomeSetsPlayerId` | welcome message sets playerId in state |
| `ReducerTest.lobbyUpdateUpdatesPlayers` | lobby_update updates player list |
| `ReducerTest.gameStartSetsPhaseActive` | game_start transitions to Active phase |
| `ReducerTest.otherPlayerViewHasNoPos` | OtherPlayerView deserialized has no position field |
| `ReconnectManagerTest.backoffDelays` | Delay sequence: 1s, 2s, 4s, 8s, 16s (cap 30s) |

### Wrong if

```
JsonDecodingException: Unexpected JSON token
```
→ Mismatch between server JSON field names and Kotlin `@Serializable` property names; use `@SerialName("field_name")` if names differ

---

## T047 — Rate Limit Enforcement

### How to test

Write a small Go test or use a script that floods the server with > 10 messages in < 1 second:

```go
// server/internal/conn/conn_test.go
func TestRateLimitClose(t *testing.T) {
    // Send 11 messages within 1 second
    // Expect connection to be closed after the 11th
}
```

Alternatively with wscat (manual):
```bash
wscat -c "ws://localhost:8080/ws?room=RATE01&name=Alice"
# Send join, then rapidly paste 11+ messages
```

### Expected

After the 11th message within 1 second, the WebSocket connection closes with status code 1008 (Policy Violation). Server logs:

```json
{"level":"WARN","msg":"rate limit exceeded","playerName":"Alice","roomId":"RATE01"}
```

### Wrong if

- Connection not closed after 11 messages → rate counter not implemented
- Connection closes after only 1 message → rate counter resets too aggressively (should be per-second window, not total)
- Server crashes instead of closing cleanly → use `wsConn.Close(websocket.StatusPolicyViolation, "rate limit")` not panic

---

## T048 — Pong Reply (Server Side)

### How to test

From an active game session, manually send a ping:
```json
{"type":"ping","ts":1715760000000,"data":{}}
```

### Expected

Within 1 second:
```json
{"type":"pong","ts":<current_epoch_ms>,"data":{}}
```

### Wrong if

- No pong received → ping message type not handled in room.go's select loop
- `ts` in pong is the same as ping's ts (echoed) → pong `ts` must be server's current time, not client's ts
- Server treats ping as an unknown action and returns error → ping must be handled before action dispatch

---

## T052 — Docker Image Size

### How to test

```powershell
cd server
docker build --no-cache -t blind-map-survival:local .
docker image inspect blind-map-survival:local --format "{{.Size}}"
```

### Expected

Output is a number less than `20971520` (20 MB in bytes).

Example of a passing result: `18234567`

### If image is too large

Check which layer is bloating:
```powershell
docker history blind-map-survival:local
```

Common causes:
- Final stage uses `golang:1.23-alpine` instead of `distroless` → distroless base is ~2 MB vs ~300 MB
- Binary not compiled with `-ldflags="-s -w"` → debug symbols not stripped; stripped binary is ~30-50% smaller
- Binary not compiled with `CGO_ENABLED=0` → binary links C runtime which can cause issues
- `COPY` copies entire directory instead of just the binary → add `.dockerignore` or use specific COPY path

### Target

Final image should contain only the static Go binary + distroless CA certificates + timezone data. Nothing else.

```dockerfile
# Expected final stage contents:
/server          # the binary
/etc/ssl/        # CA certs (from distroless)
/usr/share/zoneinfo/  # timezone (from distroless)
```

---

## T053 — Render.com Deployment

### How to test

1. Push `server/` branch to GitHub/GitLab remote
2. Render auto-detects `server/render.yaml`
3. Wait for build + deploy (first deploy ~5 min)
4. Check health endpoint:

```powershell
curl https://<your-app>.onrender.com/healthz
```

### Expected

```json
{"status":"ok"}
```

HTTP 200 within 60 seconds (accounting for cold start on free tier).

### Warm server test

Wait 2+ minutes after first request (server stays warm). Send another request:

```powershell
curl https://<your-app>.onrender.com/healthz
```

Expected: < 500ms response time.

### Cold start test

Wait 16+ minutes (free tier sleeps after 15 min idle). Send a request:

Expected: Response within 60 seconds (may take 20–30s for cold start). This simulates the "Waking server…" scenario the Android client handles.

### Wrong if

- HTTP 503 / no response → Docker image fails to start; check Render build logs
- `/healthz` takes > 100ms on a warm server → healthz is touching game state; it must return immediately without any lookup
- ENV vars not applied → check Render dashboard for envVars section; `PORT` must be `8080`

---

## T046 — Structured JSON Logging

### How to test

Start server and perform a full game session. Capture stdout:

```powershell
go run ./cmd/server 2>&1 | Tee-Object server.log
```

After a game (join, start, move, game_over), open `server.log`.

### Expected log structure

Every line must be valid JSON:
```json
{"time":"2026-05-16T10:00:00Z","level":"INFO","msg":"server listening","port":"8080"}
{"time":"2026-05-16T10:00:01Z","level":"INFO","msg":"room created","roomId":"ABC123"}
{"time":"2026-05-16T10:00:02Z","level":"INFO","msg":"player joined","roomId":"ABC123","playerName":"Alice"}
{"time":"2026-05-16T10:00:03Z","level":"INFO","msg":"player joined","roomId":"ABC123","playerName":"Bob"}
{"time":"2026-05-16T10:00:04Z","level":"INFO","msg":"game started","roomId":"ABC123","playerCount":2}
{"time":"2026-05-16T10:00:10Z","level":"DEBUG","msg":"turn resolved","roomId":"ABC123","playerName":"Alice","action":"move","durationMs":2}
{"time":"2026-05-16T10:00:20Z","level":"INFO","msg":"game ended","roomId":"ABC123","winner":"Alice","winReason":"last_alive"}
{"time":"2026-05-16T10:01:20Z","level":"INFO","msg":"room destroyed","roomId":"ABC123"}
```

### Validate JSON format

```powershell
Get-Content server.log | ForEach-Object { $_ | ConvertFrom-Json } | Select-Object level, msg
```

If any line fails to parse, it's not valid JSON.

### Wrong if

- Plain text output like `2026/05/16 10:00:00 server listening on :8080` → slog TextHandler used instead of JSONHandler
- `level` field missing → slog handler not configured with level field
- `DEBUG` messages appear when `LOG_LEVEL=info` → log level filtering not applied

---

## Full End-to-End Acceptance Test

Run this complete sequence to confirm the entire game stack works together:

1. Start server: `MAP_SIZE=5 TURN_SECONDS=30 LOG_LEVEL=info go run ./cmd/server`
2. Terminal 2: Alice joins via wscat, gets welcome
3. Terminal 3: Bob joins, both get lobby_update with 2 players
4. Alice starts game, both get game_start with individual PlayerViews
5. Alice moves N — gets turn_result; Bob gets turn_result with no Alice position
6. Bob moves S — turn advances back to Alice
7. Alice moves to bullet tile, picks up bullet
8. Alice shoots at Bob (if in line), Bob eliminated, game_over fires
9. Confirm `winReason: "last_alive"` and game ends
10. Disconnect and reconnect with Alice's original playerId mid-game — state restored
11. Stop server, verify Docker image < 20 MB
12. Deploy to Render, verify /healthz

All steps must pass for Phase 9 to be complete.

---

## Phase 9 Pass Criteria

| Test | Expected |
|---|---|
| `go vet ./...` | No output, exit code 0 |
| `go test -race ./...` | All packages pass, no DATA RACE |
| `.\gradlew :shared:test` | All Kotlin tests pass |
| Rate limit enforcement | Conn closed on 11th message in 1s |
| Pong reply | Server replies pong to client ping |
| Docker image size | < 20 MB (< 20971520 bytes) |
| Render /healthz | HTTP 200, `{"status":"ok"}` within 60s |
| Log format | Every line valid JSON with time, level, msg |
| Log level filtering | DEBUG logs absent at LOG_LEVEL=info |
| End-to-end game | Full game session completes without error |

**All 9 phases complete — Blind Map Survival is ready to ship.**
