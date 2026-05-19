# Test Guide: Phase 3 — User Story 1 (Create and Join a Private Room)

**Phase goal**: Players can create a room, others can join by code, host sees player list, server sends correct WebSocket messages. Android lobby screen connects and shows players.

**Tools needed**: `wscat` (`npm install -g wscat`), `curl`, a running server.

---

## Prerequisites

- Phase 2 complete
- Server code in `server/` compiles with `go build ./...`
- Terminal 1 running the server

---

## Start the Server

```powershell
cd server
$env:LOG_LEVEL="debug"
$env:MAP_SIZE="5"
go run ./cmd/server
```

### Expected log output

```json
{"time":"...","level":"INFO","msg":"server listening","port":"8080"}
```

### Wrong if

```
listen tcp :8080: bind: address already in use
```
→ Kill existing process on port 8080: `netstat -ano | findstr :8080` then `Stop-Process -Id <PID>`

```
panic: runtime error
```
→ Code error — read the stack trace; usually a nil pointer on startup

---

## Health Check

### How to test

```powershell
curl http://localhost:8080/healthz
```

### Expected

```json
{"status":"ok"}
```
HTTP 200 response.

### Wrong if

- Connection refused → server not running on port 8080
- `{"status":"error"}` → healthz handler has a bug; it should always return ok
- HTTP 404 → route not registered (`/healthz` handler not added in main.go)
- Response takes > 100ms → healthz is doing game state work (it must not)

---

## Single Player Connects (wscat)

### How to test

Open Terminal 2:
```bash
wscat -c "ws://localhost:8080/ws?room=TEST01&name=Alice"
```

Once connected, send:
```json
{"type":"join","ts":1715760000000,"data":{"roomCode":"TEST01","playerName":"Alice","clientVersion":"1.0.0","playerId":null}}
```

### Expected response (within 1 second)

```json
{
  "type": "welcome",
  "ts": <epoch_ms>,
  "data": {
    "playerId": "player_<alphanumeric>",
    "roomState": {
      "roomCode": "TEST01",
      "players": ["Alice"],
      "isHost": true
    }
  }
}
```

### Wrong if

- No response after 5 seconds → server is not processing the join message; check inCh reading in room.go
- `"isHost": false` for the first joiner → host assignment logic wrong; first player MUST be host
- `"players": []` → player not added to lobby before sending welcome
- `"playerId": null` → PlayerID not being generated and returned
- `"roomCode"` in welcome doesn't match query param → room lookup wrong

---

## Second Player Joins (Two wscat sessions)

### How to test

Keep Alice's session open. Open Terminal 3:
```bash
wscat -c "ws://localhost:8080/ws?room=TEST01&name=Bob"
```

Send join:
```json
{"type":"join","ts":1715760000000,"data":{"roomCode":"TEST01","playerName":"Bob","clientVersion":"1.0.0","playerId":null}}
```

### Expected — Bob receives

```json
{
  "type": "welcome",
  "ts": <epoch_ms>,
  "data": {
    "playerId": "player_<different_id>",
    "roomState": {
      "roomCode": "TEST01",
      "players": ["Alice", "Bob"],
      "isHost": false
    }
  }
}
```

### Expected — Alice receives (lobby_update)

```json
{
  "type": "lobby_update",
  "ts": <epoch_ms>,
  "data": {
    "roomCode": "TEST01",
    "players": ["Alice", "Bob"],
    "isHost": true
  }
}
```

### Wrong if

- Alice does NOT receive `lobby_update` → broadcast not implemented; `lobby_update` must go to all existing players
- Bob's `isHost` is `true` → only the first joiner (Alice) is host
- `players` list in any message is `["Alice"]` (missing Bob) → player added to state AFTER sending messages; add player first, then broadcast
- Bob gets two different playerIds in different messages → ID generation is inconsistent

---

## Room Full (9th Player Rejected)

### How to test

Create room `FULL01`. Use a script to connect 8 players sequentially (or manually). Attempt a 9th connection:

```bash
wscat -c "ws://localhost:8080/ws?room=FULL01&name=Player9"
```

Send join. 

### Expected

```json
{
  "type": "error",
  "ts": <epoch_ms>,
  "data": {
    "code": "ROOM_FULL",
    "message": "..."
  }
}
```

And the connection is closed by the server shortly after.

### Wrong if

- No error message sent → Room capacity check missing; must enforce 8-player limit
- Server crashes → nil pointer when sending to 9th conn before closing; always send error then close
- Connection stays open silently → server must close after sending ROOM_FULL

---

## Room Code Validation

### How to test

Try connecting with invalid room codes:

```bash
# Too short
wscat -c "ws://localhost:8080/ws?room=AB&name=Alice"

# Too long  
wscat -c "ws://localhost:8080/ws?room=ABCDEFG&name=Alice"

# Contains special chars
wscat -c "ws://localhost:8080/ws?room=AB-123&name=Alice"

# Missing room param
wscat -c "ws://localhost:8080/ws?name=Alice"
```

### Expected

HTTP 400 before WebSocket upgrade (connection refused during handshake).

### Wrong if

- Server accepts invalid codes → validation not applied before upgrade
- Server panics → missing nil/empty check on query params

---

## Player Name Validation

### How to test

```bash
# Empty name
wscat -c "ws://localhost:8080/ws?room=TEST01&name="

# Name > 20 chars
wscat -c "ws://localhost:8080/ws?room=TEST01&name=TwentyOneCharsLongName"
```

### Expected

HTTP 400 before upgrade.

### Wrong if

- 21-char name accepted → max length not enforced

---

## Origin Check

### How to test

```powershell
# Simulate request from disallowed origin
$headers = @{ "Origin" = "http://evil.example.com" }
Invoke-WebRequest -Uri "http://localhost:8080/ws?room=TEST01&name=Alice" -Headers $headers
```

With `ALLOWED_ORIGINS=*` (default), this SHOULD be accepted. To test rejection:

```powershell
$env:ALLOWED_ORIGINS = "http://localhost"
# restart server
# then repeat the evil origin request
```

### Expected with evil origin + restricted ALLOWED_ORIGINS

HTTP 403 Forbidden.

### Wrong if

- Accepts all origins even when ALLOWED_ORIGINS is set to a specific domain → origin validation not applied

---

## Host Promotion (Host Disconnects)

### How to test

Start room with Alice (host) + Bob. Disconnect Alice (Ctrl+C in wscat). Watch Bob's messages.

### Expected

Bob receives `lobby_update` with `"isHost": true` and `"players": ["Bob"]`.

### Wrong if

- Bob receives nothing after Alice disconnects → leave event not broadcast
- Bob's `isHost` stays `false` → host promotion logic missing
- Server crashes on Alice disconnect → nil pointer; always check conn != nil before using

---

## Room Destroyed After All Players Leave

### How to test

Start room with Alice. Disconnect Alice. Wait 5 seconds. Try to join the same room again:

```bash
wscat -c "ws://localhost:8080/ws?room=TEST01&name=Alice"
```

Send join → should create a NEW room (new playerId).

### Expected

New welcome with fresh room state (1 player, isHost=true, players=["Alice"]).

### Wrong if

- `ROOM_NOT_FOUND` error → room was destroyed but not recreated by the new join (server should auto-create if empty)
- Old player state persists → stale player data in the room map

---

## Server Logs During Test

Check server terminal. During the test sequence, logs should show:

```
level=INFO msg="room created" roomId=TEST01
level=INFO msg="player joined" roomId=TEST01 playerName=Alice
level=INFO msg="player joined" roomId=TEST01 playerName=Bob
level=INFO msg="player left" roomId=TEST01 playerName=Alice
level=INFO msg="room destroyed" roomId=TEST01
```

### Wrong if

- No logs at all → slog handler not initialized
- Logs in non-JSON format → slog handler is text not JSON

---

## Phase 3 Pass Criteria

| Test | Expected |
|---|---|
| Server starts | JSON log "server listening" on port 8080 |
| `/healthz` | `{"status":"ok"}` HTTP 200, < 100ms |
| Single player join | `welcome` with playerId + isHost=true |
| Second player join | Bob gets welcome; Alice gets lobby_update with both names |
| Lobby player list | Accurate count and names in all messages |
| Room full | 9th player gets `ROOM_FULL` error, conn closed |
| Invalid room code | HTTP 400 before upgrade |
| Host disconnect | Remaining player gets lobby_update with isHost=true |

**Proceed to Phase 4 only when all checks pass.**
