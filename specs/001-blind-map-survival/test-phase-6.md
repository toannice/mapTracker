# Test Guide: Phase 6 — User Story 4 (Eliminate Opponents via Shooting)

**Phase goal**: Shoot action consumes a bullet, traces a ray in the given direction, instantly eliminates the first player it hits. The `last_alive` win condition triggers when one player remains.

---

## Prerequisites

- Phase 5 complete and passing
- Server running: `MAP_SIZE=5 TURN_SECONDS=30 LOG_LEVEL=debug go run ./cmd/server`
- Two wscat terminals (Alice + Bob) with an active game
- At least one player has a bullet in inventory (move onto bullet tile → pickup)

---

## Setup: Known Positions for Reliable Testing

The map is random, so use server debug logs or add a temporary log in `startGame()` that prints all player starting positions. With known positions, you can navigate players into a straight line to test shooting reliably.

Alternatively: use `MAP_SIZE=5` and navigate until Alice is directly North of Bob (same X, Alice.Y < Bob.Y). Then Alice can shoot South to hit Bob.

---

## Shoot — Hit (Eliminates Opponent)

### Setup

- Alice at (x=2, y=1), Bob at (x=2, y=3) (same column, Alice North of Bob)
- Alice has 1 bullet in inventory
- It is Alice's turn

### Action (Terminal 1 — Alice)

```json
{"type":"action","ts":0,"data":{"kind":"shoot","direction":"S"}}
```

### Expected — Alice receives `turn_result`

```json
{
  "type": "turn_result",
  "data": {
    "self": {
      "inventory": [],
      ...
    },
    "others": [{"id":"<bob_id>","name":"Bob","alive":false}],
    "events": [
      {
        "kind": "player_eliminated",
        "payload": {"playerName":"Bob","byPlayerName":"Alice"}
      }
    ],
    "phase": "active"
  }
}
```

Bullet is consumed: Alice's `inventory` is now empty.
Bob shows `alive: false` in Alice's `others` list.

### Expected — Bob receives `turn_result`

```json
{
  "type": "turn_result",
  "data": {
    "others": [{"id":"<alice_id>","name":"Alice","alive":true}],
    "events": [
      {
        "kind": "player_eliminated",
        "payload": {"playerName":"Bob","byPlayerName":"Alice"}
      }
    ]
  }
}
```

### Wrong if

- Alice's `inventory` still contains bullet after shoot → bullet not consumed
- `player_eliminated` event missing → elimination not generated
- `player_eliminated` sent only to Alice → this event must go to ALL players
- Bob's `alive` stays `true` → `Player.Alive` not set to false
- Alice's shoot action doesn't consume turn → turn must advance after shoot

---

## Shoot — Miss (No Player in Path)

### Setup

Alice at (x=2, y=2), Bob at (x=0, y=0) (not in Alice's shooting path). Alice has 1 bullet.

Alice shoots East:
```json
{"type":"action","ts":0,"data":{"kind":"shoot","direction":"E"}}
```

### Expected — Alice receives `turn_result`

```json
{
  "self": {
    "inventory": []
  },
  "events": [],
  "others": [{"name":"Bob","alive":true}]
}
```

Bullet consumed. No elimination. Bob still alive.

### Wrong if

- Bullet NOT consumed on miss → bullet must always be consumed, hit or miss (FR-017)
- `player_eliminated` event fires for Bob even though bullet missed → ray trace wrong; check cell-by-cell traversal

---

## Last-Alive Win Condition

### Setup

2-player game. One player has a bullet and a clear shot at the other.

Eliminate the second player. With only Alice remaining:

### Expected — both players receive `game_over`

```json
{
  "type": "game_over",
  "ts": <epoch_ms>,
  "data": {
    "winner": "Alice",
    "winReason": "last_alive"
  }
}
```

Game phase transitions to `"ended"`.

After `game_over`, no further actions are accepted:
```json
{"type":"error","data":{"code":"WRONG_PHASE","message":"..."}}
```

### Wrong if

- `game_over` not sent after elimination → `checkWinCondition` not called after shoot
- `winReason` is `"map_complete"` → wrong win reason for last-alive
- `winner` is null → winner field not populated; should be the surviving player's name
- Game continues after `game_over` → phase not set to PhaseEnded; must reject further actions

---

## Shoot — NO_BULLET Error

Attempt to shoot without any bullet in inventory:

```json
{"type":"action","ts":0,"data":{"kind":"shoot","direction":"N"}}
```

### Expected

```json
{"type":"error","data":{"code":"NO_BULLET","message":"..."}}
```

Turn does NOT advance. Player must still have a valid turn.

### Wrong if

- No error returned → inventory check missing
- Turn advances even though shoot failed → errors must not consume the turn

---

## Shoot — INVALID_DIRECTION Error

Send shoot with invalid direction:

```json
{"type":"action","ts":0,"data":{"kind":"shoot","direction":"X"}}
```

### Expected

```json
{"type":"error","data":{"code":"INVALID_DIRECTION","message":"..."}}
```

### Wrong if

- Server crashes → input validation missing; `direction` must be validated before any processing
- Bullet consumed before error is returned → validate first, consume later

---

## Ray Trace: Multiple Players in Path

### Setup

3-player game: Alice at (2,0), Bob at (2,2), Carol at (2,4) — all same column.
Alice shoots South.

### Expected

Only Bob is eliminated (first player in path). Carol stays alive.

```json
"events": [
  {"kind":"player_eliminated","payload":{"playerName":"Bob","byPlayerName":"Alice"}}
]
```

Carol receives the event but `others` shows Carol as alive.

### Wrong if

- Both Bob AND Carol eliminated → ray trace doesn't stop at first hit; must stop at first player
- Neither eliminated → ray hits wall without checking for players first

---

## Shoot Through Empty Cells (Ray Boundary)

Player at (x=0, y=2) shoots West:

```json
{"type":"action","ts":0,"data":{"kind":"shoot","direction":"W"}}
```

x=0, so no cells exist to the West. Bullet travels 0 cells.

### Expected

Bullet consumed, no event, turn advances.

### Wrong if

- Server panics → `x-1 = -1` array access; boundary check must be before array lookup
- Turn doesn't advance → successful (if uneventful) shoot must still consume turn

---

## Eliminated Player Cannot Act

After Bob is eliminated, if Bob's turn comes around (if game hasn't ended):

Server must skip Bob's turn automatically (skip dead players in TurnOrder).

### Expected

`turn_skipped` event with Bob's name skipped automatically without waiting for timer.

### Wrong if

- Dead player's turn waits for timer → dead players must be skipped immediately in TurnOrder advancement
- Dead player can still send actions → `Alive=false` check missing in action dispatch

---

## Room Destroyed After Game Over

After `game_over` is sent, wait 60 seconds (or check server logs for "room destroyed"). Then try to reconnect to the same room:

```bash
wscat -c "ws://localhost:8080/ws?room=GAME01&name=Alice"
```

Send join → creates a NEW fresh room.

### Wrong if

- Room not destroyed after 60s → cleanup goroutine not started after game_over
- Old game state persists → stale GameState in Hub map

---

## 3-Player Game: Last-Alive with 1 Alive

3-player game, Alice eliminates Bob, then eliminates Carol.

After Carol's elimination:

```json
{
  "type": "game_over",
  "data": {
    "winner": "Alice",
    "winReason": "last_alive"
  }
}
```

### Wrong if

- `game_over` fires after Bob's elimination (with Carol still alive) → check counts living players correctly

---

## Phase 6 Pass Criteria

| Test | Expected |
|---|---|
| Shoot hit | Bullet consumed, target alive=false, player_eliminated event to all |
| Shoot miss | Bullet consumed, no elimination, turn advances |
| Last-alive win | game_over to all players with winReason="last_alive" |
| NO_BULLET error | Error returned, turn not consumed |
| INVALID_DIRECTION | Error returned, bullet not consumed |
| Ray stops at first hit | Only first player in path eliminated |
| Boundary shot | Bullet consumed, no crash |
| Dead player turn | Auto-skipped, not waiting for timer |
| Post-game actions | WRONG_PHASE error |

**Proceed to Phase 7 only when all checks pass.**
