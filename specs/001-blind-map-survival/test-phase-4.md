# Test Guide: Phase 4 — User Story 2 (Take Turns and Navigate the Map)

**Phase goal**: The active-game turn loop works — Move/Pickup actions update player state, per-player filtered `turn_result` is sent, timer auto-skips idle players, and opponent positions are never leaked.

**Tools needed**: Two wscat terminals, server running with `MAP_SIZE=5` for manageable grid.

---

## Prerequisites

- Phase 3 complete and passing
- Server running: `MAP_SIZE=5 TURN_SECONDS=10 LOG_LEVEL=debug go run ./cmd/server`
  - Use `TURN_SECONDS=10` so auto-skip is testable without waiting 30 seconds

---

## Setup: Start a 2-Player Game

**Terminal 1 (Alice — host)**:
```bash
wscat -c "ws://localhost:8080/ws?room=GAME01&name=Alice"
```
Send join, note `playerId` from welcome.

**Terminal 2 (Bob)**:
```bash
wscat -c "ws://localhost:8080/ws?room=GAME01&name=Bob"
```
Send join, note `playerId`.

**Alice starts the game** (send from Terminal 1):
```json
{"type":"action","ts":1715760000000,"data":{"kind":"start_game"}}
```

### Expected — both players receive `game_start`

Alice's game_start (example):
```json
{
  "type": "game_start",
  "ts": <epoch_ms>,
  "data": {
    "self": {
      "id": "player_alice_id",
      "name": "Alice",
      "pos": {"x": 2, "y": 3},
      "alive": true,
      "inventory": [],
      "visitedCount": 1,
      "totalCells": 25,
      "infoBlackout": false
    },
    "others": [{"id": "player_bob_id", "name": "Bob", "alive": true}],
    "visibleMap": [{"pos": {"x": 2, "y": 3}, "kind": "empty"}],
    "events": [],
    "turnEndsAt": <epoch_ms + 10000>,
    "currentTurn": "<one of the two playerIds>",
    "turn": 1,
    "phase": "active"
  }
}
```

### Wrong if

- `self.pos` is `{"x":0,"y":0}` for both players → starting positions not randomized (or not assigned)
- `others[0]` has a `pos` field → position leak; `OtherPlayerView` must not include pos
- `totalCells` is not 25 (for MAP_SIZE=5) → MapSize not propagated to SelfView
- `visitedCount` is 0 → starting cell not added to VisitedCells on game start
- `phase` is `"lobby"` → phase transition didn't happen

---

## Move Action (Valid)

Determine whose turn it is from `currentTurn`. That player sends a Move action.

If it's **Alice's turn** (Terminal 1):
```json
{"type":"action","ts":1715760000000,"data":{"kind":"move","direction":"N"}}
```

### Expected — Alice receives `turn_result`

```json
{
  "type": "turn_result",
  "data": {
    "self": {
      "pos": {"x": 2, "y": 2},
      "visitedCount": 2,
      ...
    },
    "currentTurn": "<Bob's playerID>",
    "turn": 2,
    "phase": "active"
  }
}
```

Alice's y-coordinate decreases by 1 (N = y-1). `visitedCount` becomes 2.

### Expected — Bob receives `turn_result`

```json
{
  "type": "turn_result",
  "data": {
    "self": {
      "pos": {"x": <Bob's pos>},
      ...
    },
    "others": [{"id": "player_alice_id", "name": "Alice", "alive": true}],
    "currentTurn": "<Bob's playerID>",
    ...
  }
}
```

Bob's `others` list shows Alice as alive, **but has no `pos` field**.

### Wrong if

- Alice's `pos` doesn't change after move N → `applyMove` not updating `player.Pos`
- Alice's `visitedCount` stays 1 → new cell not added to `VisitedCells`
- Bob's `turn_result` contains Alice's position → critical security bug; `OtherPlayerView` must never include pos
- `currentTurn` does not advance to Bob → `TurnOrder` index not incremented

---

## Direction Coordinate System

Verify all four directions work correctly on a 5×5 grid (positions 0-4):

| Direction | Effect on pos |
|---|---|
| N (North) | y - 1 |
| S (South) | y + 1 |
| E (East) | x + 1 |
| W (West) | x - 1 |

Send each direction and confirm the coordinate changes. The spec does not specify visual orientation — what matters is that N/S/E/W each modify a different axis consistently.

### Wrong if

- N and S both decrease y → axis assignment error
- Moving E changes y instead of x → wrong axis for direction

---

## Boundary Rejection

Move a player to position (x=0, y=0). Attempt to move W (x would become -1):

```json
{"type":"action","ts":1715760000000,"data":{"kind":"move","direction":"W"}}
```

### Expected

```json
{
  "type": "error",
  "data": {
    "code": "INVALID_DIRECTION",
    "message": "..."
  }
}
```

The player's position does NOT change. The turn does NOT advance (same player must act again).

### Wrong if

- Position becomes `{"x":-1,"y":0}` → boundary check missing; grid is 0 to mapSize-1
- Turn advances even though move was invalid → boundary errors must not consume the turn
- No error message sent → client left waiting with no feedback

---

## NOT_YOUR_TURN Error

While it is Bob's turn, Alice sends an action:

```json
{"type":"action","ts":1715760000000,"data":{"kind":"move","direction":"N"}}
```

### Expected — Alice receives

```json
{
  "type": "error",
  "data": {
    "code": "NOT_YOUR_TURN",
    "message": "It is Bob's turn."
  }
}
```

Bob's turn is NOT interrupted. `currentTurn` stays on Bob.

### Wrong if

- Alice's action is applied → game state corrupted; only the current player may act
- No error returned to Alice → Alice's client gets stuck with no feedback

---

## Pickup from Bullet Tile

Find a bullet tile on the map (check server debug logs or visit cells until `kind: "bullet"` appears in `visibleMap`). Move the active player onto it. Send:

```json
{"type":"action","ts":1715760000000,"data":{"kind":"pickup"}}
```

### Expected

```json
{
  "type": "turn_result",
  "data": {
    "self": {
      "inventory": [{"id": "item_<uuid>", "kind": "bullet"}],
      ...
    }
  }
}
```

The bullet tile's `kind` in `visibleMap` remains `"bullet"` (infinite pickups — tile does NOT disappear).

### Wrong if

- `inventory` stays empty → `applyPickup` not adding Item to Inventory
- Tile disappears from `visibleMap` after pickup → tile should be permanent; fix CellKind after pickup
- Second pickup on same tile gives error → FR-010: bullet tiles have unlimited pickups

---

## Pickup on Empty Cell

Move the active player to a cell with `kind: "empty"`. Send pickup:

```json
{"type":"action","ts":1715760000000,"data":{"kind":"pickup"}}
```

### Expected

```json
{
  "type": "error",
  "data": {
    "code": "NOTHING_TO_PICKUP",
    "message": "..."
  }
}
```

The turn does NOT advance.

### Wrong if

- No error → pickup on empty cell accepted; must reject with NOTHING_TO_PICKUP
- Turn advances after rejection → errors must not consume turn

---

## Turn Timer Auto-Skip

Wait without sending any action until the turn timer expires (10 seconds with `TURN_SECONDS=10`).

### Expected — both players receive an event

```json
{
  "type": "turn_result",
  "data": {
    "events": [
      {
        "kind": "turn_skipped",
        "payload": {"playerName": "<current player>", "reason": "timeout"}
      }
    ],
    "currentTurn": "<next player's ID>",
    "turn": <N+1>,
    "phase": "active"
  }
}
```

### Wrong if

- No auto-skip after 10s → 100ms ticker not implemented or `TurnDeadline` check failing
- Skip fires immediately at game start → `TurnDeadline` set to `time.Time{}` (zero value); must be `time.Now().Add(...)`
- Both players' turns get skipped simultaneously → only one turn advances per tick cycle

---

## No Position Leak — Network-Level Check

After each `turn_result`, manually inspect the raw JSON Bob receives. Search for Alice's position coordinates (look up Alice's position from her own turn_result). Bob's messages must never contain those exact x,y values in the `others` array.

### Checklist

Open server logs with `LOG_LEVEL=debug`. Each `turn_result` broadcast line in the log should note `recipient=<playerID>`. Confirm two separate log lines per turn (one for Alice, one for Bob) with different payloads.

### Wrong if

- Only one log line per turn → single broadcast used instead of per-player BuildPlayerView
- Both log lines have identical payload → filtering not applied; must generate separate views

---

## VisitedMap Grows Correctly

After Alice moves 3 times to distinct cells:

```json
"visibleMap": [
  {"pos": {"x":2,"y":3}, "kind":"empty"},
  {"pos": {"x":2,"y":2}, "kind":"empty"},
  {"pos": {"x":2,"y":1}, "kind":"empty"}
]
```

### Wrong if

- `visibleMap` only shows current cell → `VisitedCells` being replaced instead of added to
- `visibleMap` shows ALL 25 cells → no filtering; only visited cells should appear
- `visitedCount` doesn't match `len(visibleMap)` → count not maintained correctly

---

## WRONG_PHASE — Action During Lobby

While game is still in lobby (before host starts), Bob sends:
```json
{"type":"action","ts":1715760000000,"data":{"kind":"move","direction":"N"}}
```

### Expected

```json
{"type":"error","data":{"code":"WRONG_PHASE","message":"..."}}
```

### Wrong if

- Action processed during lobby → phase check missing in action dispatch

---

## Phase 4 Pass Criteria

| Test | Expected |
|---|---|
| game_start received by both | Both players get individual PlayerView |
| Alice's pos hidden from Bob | No pos field in others array |
| Move N changes y by -1 | Coordinate system consistent |
| Boundary rejection | INVALID_DIRECTION error, turn not consumed |
| NOT_YOUR_TURN | Error to wrong-turn player, game continues |
| Bullet pickup | Item added to inventory; tile remains |
| Empty cell pickup | NOTHING_TO_PICKUP error |
| Auto-skip on timeout | turn_skipped event after TURN_SECONDS |
| visitedMap grows | Only visited cells visible; count matches |
| WRONG_PHASE | Error if action sent during lobby |

**Proceed to Phase 5 only when all checks pass.**
