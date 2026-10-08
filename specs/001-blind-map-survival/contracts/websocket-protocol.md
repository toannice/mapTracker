# WebSocket Protocol Contract

**Version**: 2.0 (Phase 2 — see `../phase2.md`)
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

For **reconnect** (resuming a session), set `playerId` to the original player ID returned in `welcome`
**and** `reconnectToken` to the token from that same `welcome`. Player IDs are public (every
`PlayerView` lists them), so the server ignores a `playerId` that does not come with its owner's
token and treats the request as a fresh join (`GAME_IN_PROGRESS` once a match is running).

In practice clients pass these as query parameters on the upgrade URL —
`/ws?room=ABC123&name=Alice&playerId=…&token=…` — and the server builds the `join` from them.

---

### `action` (during Active phase)
Most actions consume a turn (`move`, `shoot`). `submit_map` does not. The server
rejects turn-consuming actions sent out of turn or in the wrong phase.

**Move**:
```json
{
  "type": "action",
  "ts": 1715760000000,
  "data": { "kind": "move", "direction": "N" }
}
```
`direction` must be one of: `"N"`, `"S"`, `"E"`, `"W"`.

Moving into the map border or an interior wall does **not** error — it emits a
`player_moved` event with `success: false` and still consumes the turn.
Moving onto a `bullet` cell auto-grants a bullet (capped at 1; no `pickup`
action exists). There is no `pickup` action.

**Shoot**:
```json
{
  "type": "action",
  "ts": 1715760000000,
  "data": { "kind": "shoot", "direction": "E" }
}
```
Requires player to have at least one bullet in inventory. Bullet travels until hitting a player (instant elimination) or the map boundary.

**Submit Map** (does NOT consume a turn — allowed any time during Active phase):
```json
{
  "type": "action",
  "ts": 1715760000000,
  "data": { "kind": "submit_map", "walls": [ {"x":1,"y":1}, {"x":2,"y":3} ] }
}
```
`walls` is the set of cells the player believes are walls. The server compares
it to the real wall set:
- **Exact match** → that player wins (`winReason: "map_complete"`).
- **Mismatch** → the player's `submitsLeft` decrements; a `map_submitted` event
  is broadcast to everyone with the `wrong` count (symmetric difference).

`submitsLeft` starts at 3. At 0, `submit_map` returns `error` `NO_SUBMIT_LEFT`.

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
Sent after successful join/rejoin. `reconnectToken` is a secret sent only to its owner; keep it for
the session and present it on reconnect.
```json
{
  "type": "welcome",
  "ts": 1715760000000,
  "data": {
    "playerId": "player_abc123",
    "reconnectToken": "9f2c…",
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
              "inventory": [], "visitedCount": 1, "totalCells": 400,
              "infoBlackout": false, "submitsLeft": 3 },
    "others": [
      { "id": "...", "name": "Bob", "alive": true }
    ],
    "visibleMap": [ { "pos": {"x":3,"y":7}, "kind": "empty" } ],
    "events": [],
    "turnEndsAt": 1715760030000,
    "currentTurn": "<playerIdOfFirstPlayer>",
    "turn": 1,
    "phase": "active",
    "mapStats": {
      "mapSize": 20,
      "counts": { "wall": 100, "blank": 250, "trap": 12, "reward": 20, "bullet": 40, "portal": 4 }
    }
  }
}
```
`mapStats` is aggregate-only (cell-kind counts, no positions) — shared knowledge
for all players, backing the client "Info" button.

---

### `turn_result`
Sent to each player individually (filtered view) after every turn resolves.

Payload is identical to `game_start` but reflects updated state after the turn.
The `events` array holds what happened this turn. Events are **broadcast to
every player** (each sees the same list) so all actions are public knowledge —
the one exception is `clue_received`, which is suppressed for a player under
`infoBlackout`. The server sends only the current turn's events; the client
accumulates history itself.

---

### Events (entries in `turn_result.events`)

Each entry is `{ "kind": "<kind>", "payload": { ... } }`. The standalone `event`
message type is reserved but unused — events travel inside `turn_result`.

**Player moved** — emitted for every move, success or failure. Carries **no
`pos`** (coordinates are never revealed for normal moves):
```json
{ "kind": "player_moved",
  "payload": { "playerName": "Alice", "direction": "up", "success": true,
               "blockType": "bullet", "bulletFull": false } }
```
`direction` is `up`/`down`/`left`/`right`. `success: false` → hit a wall/border
(no `blockType`). `blockType` ∈ `blank`/`bullet`/`reward`/`trap`/`portal`.

**Clue received** (compass or reward) — private; suppressed under `infoBlackout`:
```json
{ "kind": "clue_received", "payload": { "type": "nearest_direction", "direction": "NE" } }
```

**Player eliminated**:
```json
{ "kind": "player_eliminated", "payload": { "playerName": "Bob", "byPlayerName": "Alice" } }
```

**Shot fired**:
```json
{ "kind": "shot_fired",
  "payload": { "byPlayerId": "...", "byPlayerName": "Alice", "direction": "right" } }
```

**Trap triggered**:
```json
{ "kind": "trap_triggered", "payload": { "effect": "lose_next_turn" } }
```
`effect` ∈ `reveal_position` (keeps `pos`), `random_teleport`, `lose_next_turn`,
`lose_bullet`, `info_blackout`.

**Reward activated**:
```json
{ "kind": "reward_activated", "payload": { "effect": "nearest_direction", "direction": "SW" } }
```
`effect` ∈ `all_positions_revealed` (keeps positions), `nearest_direction`,
`all_bullet_locations` (keeps locations).

**Map submitted** — broadcast after any `submit_map`:
```json
{ "kind": "map_submitted",
  "payload": { "playerName": "Alice", "correct": false, "wrong": 4, "submitsLeft": 2 } }
```

**Turn skipped** (timeout or skip-next-turn trap):
```json
{ "kind": "turn_skipped", "payload": { "playerId": "...", "playerName": "Carol" } }
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
| `NOT_YOUR_TURN` | Turn-consuming action sent when it is another player's turn |
| `INVALID_DIRECTION` | Direction not in {N,S,E,W} |
| `NO_BULLET` | Shoot action with no bullet in inventory |
| `NO_SUBMIT_LEFT` | `submit_map` when `submitsLeft` has reached 0 |
| `ROOM_NOT_FOUND` | Room code does not exist |
| `ROOM_FULL` | Room already has 8 players |
| `WRONG_PHASE` | Action sent during Lobby or Ended phase |

Note: hitting a wall/border is **not** an error — it is a normal `player_moved`
event with `success: false`. `pickup` and `MAP_INCOMPLETE` no longer exist.

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
4. Server MUST NOT trust `playerId` in `data` for any purpose other than rejoin lookup, and MUST
   only resume a player when the matching `reconnectToken` is presented with it.
5. Server MUST enforce WSS only in production (Render provides TLS termination).
