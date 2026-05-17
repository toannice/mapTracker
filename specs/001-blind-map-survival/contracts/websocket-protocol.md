# WebSocket Protocol Contract

**Version**: 1.0
**Transport**: WebSocket (WSS only in production, WS allowed on localhost)
**Endpoint**: `GET /ws?room=<roomCode>&name=<playerName>`
**Format**: JSON text frames
**Envelope**: Every message (both directions) uses `{"type":"<type>","ts":<epoch_ms>,"data":{...}}`

---

## Upgrade Request

```
GET /ws HTTP/1.1
Host: <server>
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: <key>

Query params:
  room  — 6-char alphanumeric room code (required)
  name  — player display name (required, 1–20 chars, trimmed)
```

---

## Client → Server Messages

### `join`
Sent immediately after WebSocket upgrade. Identifies the player to the room.

```json
{
  "type": "join",
  "ts": 1715760000000,
  "data": {
    "roomCode": "ABC123",
    "playerName": "Alice",
    "clientVersion": "1.0.0",
    "playerId": null
  }
}
```

For **reconnect** (resuming a session), set `playerId` to the original player ID returned in `welcome`.

---

### `action` (during Active phase)
One action per turn. The server rejects actions sent out of turn or in wrong phase.

**Move**:
```json
{
  "type": "action",
  "ts": 1715760000000,
  "data": { "kind": "move", "direction": "N" }
}
```
`direction` must be one of: `"N"`, `"S"`, `"E"`, `"W"`.

**Pickup** (picks up item on current cell):
```json
{
  "type": "action",
  "ts": 1715760000000,
  "data": { "kind": "pickup" }
}
```
Fails silently if cell has no pickable item. Server sends `error` event.

**Shoot**:
```json
{
  "type": "action",
  "ts": 1715760000000,
  "data": { "kind": "shoot", "direction": "E" }
}
```
Requires player to have at least one bullet in inventory. Bullet travels until hitting a player (instant elimination) or the map boundary.

**Submit Map**:
```json
{
  "type": "action",
  "ts": 1715760000000,
  "data": { "kind": "submit_map" }
}
```
Server validates that `len(player.VisitedCells) == mapSize * mapSize`. If invalid, returns `error` with remaining count.

---

### `ping` (heartbeat)
Sent every 20 seconds by the client.
```json
{ "type": "ping", "ts": 1715760000000, "data": {} }
```
If no `pong` is received within 10 seconds, client initiates reconnect.

---

## Server → Client Messages

### `welcome`
Sent after successful join/rejoin.
```json
{
  "type": "welcome",
  "ts": 1715760000000,
  "data": {
    "playerId": "player_abc123",
    "roomState": {
      "roomCode": "ABC123",
      "players": ["Alice", "Bob"],
      "isHost": true
    }
  }
}
```

---

### `lobby_update`
Sent to all players when someone joins or leaves during Lobby phase.
```json
{
  "type": "lobby_update",
  "ts": 1715760000000,
  "data": {
    "roomCode": "ABC123",
    "players": ["Alice", "Bob", "Carol"],
    "isHost": false
  }
}
```

---

### `game_start`
Sent to all players when host starts the game.
```json
{
  "type": "game_start",
  "ts": 1715760000000,
  "data": {
    "self": { "id": "...", "name": "Alice", "pos": {"x":3,"y":7}, "alive": true,
              "inventory": [], "visitedCount": 1, "totalCells": 400, "infoBlackout": false },
    "others": [
      { "id": "...", "name": "Bob", "alive": true }
    ],
    "visibleMap": [ { "pos": {"x":3,"y":7}, "kind": "empty" } ],
    "events": [],
    "turnEndsAt": 1715760030000,
    "currentTurn": "<playerIdOfFirstPlayer>",
    "turn": 1,
    "phase": "active"
  }
}
```

---

### `turn_result`
Sent to each player individually (filtered view) after every turn resolves.

Payload is identical to `game_start` but reflects updated state after the turn.

---

### `event`
Discrete game notifications — scoped to specific recipient(s).

**Clue received** (compass or reward):
```json
{
  "type": "event",
  "ts": 1715760000000,
  "data": {
    "kind": "clue_received",
    "payload": {
      "clueType": "nearest_direction",
      "value": "NE"
    }
  }
}
```

**Player eliminated**:
```json
{
  "type": "event",
  "ts": 1715760000000,
  "data": {
    "kind": "player_eliminated",
    "payload": { "playerName": "Bob", "byPlayerName": "Alice" }
  }
}
```
Sent to all players.

**Trap triggered** (sent only to the trapped player):
```json
{
  "type": "event",
  "ts": 1715760000000,
  "data": {
    "kind": "trap_triggered",
    "payload": { "effect": "lose_next_turn" }
  }
}
```
`effect` is one of: `"reveal_position"`, `"random_teleport"`, `"lose_next_turn"`, `"lose_bullet"`, `"info_blackout"`.

**Reward activated**:
```json
{
  "type": "event",
  "ts": 1715760000000,
  "data": {
    "kind": "reward_activated",
    "payload": { "effect": "nearest_direction", "value": "SW" }
  }
}
```
`effect` is one of: `"all_positions_revealed"`, `"nearest_direction"`, `"all_bullet_locations"`.

**Turn skipped** (auto-skip on timeout or skip-next-turn trap):
```json
{
  "type": "event",
  "ts": 1715760000000,
  "data": { "kind": "turn_skipped", "payload": { "playerName": "Carol", "reason": "timeout" } }
}
```

---

### `game_over`
Sent to all players when the game ends.
```json
{
  "type": "game_over",
  "ts": 1715760000000,
  "data": {
    "winner": "Alice",
    "winReason": "last_alive"
  }
}
```
`winReason` is one of: `"last_alive"`, `"map_complete"`. `winner` is `null` if all players disconnected.

---

### `error`
Sent only to the requesting client for invalid actions.
```json
{
  "type": "error",
  "ts": 1715760000000,
  "data": {
    "code": "NOT_YOUR_TURN",
    "message": "It is Bob's turn."
  }
}
```

**Error codes**:
| Code | Trigger |
|------|---------|
| `NOT_YOUR_TURN` | Action sent when it is another player's turn |
| `INVALID_DIRECTION` | Direction not in {N,S,E,W} |
| `NO_BULLET` | Shoot action with no bullet in inventory |
| `NOTHING_TO_PICKUP` | Pickup action on empty cell |
| `MAP_INCOMPLETE` | Submit Map when not all cells visited; includes `remaining` count |
| `ROOM_NOT_FOUND` | Room code does not exist |
| `ROOM_FULL` | Room already has 8 players |
| `WRONG_PHASE` | Action sent during Lobby or Ended phase |

---

### `pong`
Reply to client `ping`.
```json
{ "type": "pong", "ts": 1715760000000, "data": {} }
```

---

### `server_shutdown`
Sent before graceful restart (SIGTERM → ~10s grace).
```json
{
  "type": "server_shutdown",
  "ts": 1715760000000,
  "data": { "reconnectAfterMs": 5000 }
}
```
Client should begin reconnect after `reconnectAfterMs` milliseconds.

---

## Security Rules

1. Server MUST close connection if client sends > 10 messages/second.
2. Server MUST validate `Origin` header on upgrade request (whitelist: `localhost` + production domain).
3. Server MUST validate all incoming action fields; unknown fields are silently ignored.
4. Server MUST NOT trust `playerId` in `data` for any purpose other than rejoin lookup.
5. Server MUST enforce WSS only in production (Render provides TLS termination).
