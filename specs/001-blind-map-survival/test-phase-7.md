# Test Guide: Phase 7 — User Story 5 (Win by Mapping the Entire Board)

**Phase goal**: The `submit_map` action validates server-side VisitedCells. Full map → `game_over{winReason:"map_complete"}`. Partial map → `MAP_INCOMPLETE` error with remaining count.

**Key**: Use `MAP_SIZE=3` (9 cells) for fast testing — visiting all 25 cells of a 5×5 map takes too long for manual testing.

---

## Prerequisites

- Phase 6 complete and passing
- Server running: `MAP_SIZE=3 TURN_SECONDS=30 LOG_LEVEL=debug go run ./cmd/server`
- Single player game is fine for map submission tests (2 players required by server, but one can be idle)

---

## Setup: MAP_SIZE=3 (9 Cells)

Start server with:
```powershell
$env:MAP_SIZE = "3"
$env:TURN_SECONDS = "60"
go run ./cmd/server
```

This creates a 3×3 grid with positions (0,0) through (2,2). `totalCells = 9`.

Start a 2-player game (Alice + idle Bob). Alice will be the one visiting cells.

From `game_start`, Alice's starting position counts as 1 visited cell.

---

## Visit All 9 Cells

From Alice's starting position, navigate to visit every cell. The 3×3 grid has 9 cells — Alice starts at 1, needs to visit 8 more.

**Strategy for 3×3 grid** (assuming start at (0,0)):
```
Move E → (1,0)
Move E → (2,0)
Move S → (2,1)
Move W → (1,1)
Move W → (0,1)
Move S → (0,2)
Move E → (1,2)
Move E → (2,2)
```

After each move, check `self.visitedCount` in `turn_result`. After 9 moves total, `visitedCount` should be 9 and `visibleMap` should have 9 entries.

**Note**: Bob still takes turns. Skip Bob's turns by waiting for auto-skip or have Bob send any valid action.

After Alice has visited all 9 cells:

```json
"self": {
  "visitedCount": 9,
  "totalCells": 9,
  "pos": {"x":2,"y":2}
}
```

---

## Submit Map — Full Map (Success)

When it is Alice's turn and `visitedCount == totalCells == 9`, send:

```json
{"type":"action","ts":0,"data":{"kind":"submit_map"}}
```

### Expected — all players receive `game_over`

```json
{
  "type": "game_over",
  "ts": <epoch_ms>,
  "data": {
    "winner": "Alice",
    "winReason": "map_complete"
  }
}
```

Game ends immediately. No further actions accepted.

### Wrong if

- No response → `applySubmitMap` not wired in action dispatch switch
- `WRONG_PHASE` error → submit_map handler checking wrong condition
- `winReason: "last_alive"` → wrong win reason; should be `"map_complete"`
- `winner: null` → winner field not set; should be the submitting player's name
- Game_over NOT sent to Bob → must broadcast to all players

---

## Submit Map — Partial Map (Rejection)

Start a fresh game with MAP_SIZE=3. Move Alice to only 5 out of 9 cells. Then send submit_map:

```json
{"type":"action","ts":0,"data":{"kind":"submit_map"}}
```

### Expected

```json
{
  "type": "error",
  "ts": <epoch_ms>,
  "data": {
    "code": "MAP_INCOMPLETE",
    "message": "4 cells remain"
  }
}
```

The turn does NOT advance. Alice must still complete mapping before trying again.

### Wrong if

- Game ends with partial map → validation check not applied; `len(player.VisitedCells)` must equal `state.MapSize * state.MapSize`
- Error says "0 cells remain" but game doesn't end → off-by-one in remaining count calculation
- Turn advances after MAP_INCOMPLETE → errors must not consume the turn; Alice still has her turn

---

## Remaining Count Accuracy

Test at exactly 1 cell remaining:

- Visit 8 out of 9 cells. Submit map.

### Expected

```json
{"code":"MAP_INCOMPLETE","message":"1 cells remain"}
```

Then visit the last cell. Submit map → `game_over`.

### Wrong if

- "0 cells remain" error when 1 cell is unvisited → `total - len(visited)` calculation off-by-one
- "2 cells remain" when 1 is unvisited → count wrong

---

## VisitedCells Not Double-Counted

Visit the same cell twice (go to (1,0), then back to (0,0), then to (1,0) again).

`visitedCount` must NOT increase when revisiting. `visibleMap` must NOT have duplicate entries for (1,0).

### Check

After revisiting (1,0) twice:
- `visitedCount` = 2 (start cell + (1,0)) — same as after first visit
- `visibleMap` has exactly 2 entries

### Wrong if

- `visitedCount = 3` after revisiting same cell → `VisitedCells` is a slice not a map; must be `map[Position]bool` (deduplicates automatically)
- `visibleMap` has 3 entries with duplicate (1,0) → duplicate entries indicate map rather than set semantics broken

---

## submit_map During Wrong Phase

Send `submit_map` while still in lobby:

```json
{"type":"action","ts":0,"data":{"kind":"submit_map"}}
```

### Expected

```json
{"type":"error","data":{"code":"WRONG_PHASE","message":"..."}}
```

### Wrong if

- Server crashes → phase check missing before action dispatch

---

## submit_map from Non-Active Player (NOT_YOUR_TURN)

During Bob's turn, Alice sends `submit_map`:

### Expected

```json
{"type":"error","data":{"code":"NOT_YOUR_TURN","message":"..."}}
```

### Wrong if

- Alice's submission is processed out of turn → turn ownership not checked for submit_map

---

## Sequential Turn — Only One Winner

Two players, both racing to map full board. (Turns are sequential, not simultaneous.)

Alice finishes first (on her turn, visits last cell, submits). Bob has not submitted yet.

### Expected

Alice gets `game_over{winner:"Alice",winReason:"map_complete"}` immediately. Bob also gets `game_over`. Neither can act further.

Per the spec clarification (Q3): simultaneous submission is impossible because turns are strictly sequential. This test confirms the sequential nature prevents ties.

### Wrong if

- Both players get `winner: null` → race condition in game_over detection; only one player can win

---

## visitedCount in SelfView Matches Server State

After every move, verify:
`self.visitedCount == len(self.visibleMap)`

These two values must always be equal. If they differ, the server is tracking state inconsistently.

### Wrong if

`visitedCount = 5` but `len(visibleMap) = 3` → `visitedCount` updated from `len(VisitedCells)` but `visibleMap` built from a different source; both must come from `player.VisitedCells`

---

## Phase 7 Pass Criteria

| Test | Expected |
|---|---|
| Full map submit (MAP_SIZE=3, 9 cells) | game_over winReason="map_complete" |
| Partial map submit | MAP_INCOMPLETE error with correct remaining count |
| 1 cell remaining | "1 cells remain" error |
| Turn not consumed on error | Same player can retry after MAP_INCOMPLETE |
| Cell revisit | visitedCount not incremented; no duplicate in visibleMap |
| WRONG_PHASE | Error if submitted during lobby |
| NOT_YOUR_TURN | Error if wrong player submits |
| Sequential win | Only first submitter wins; no tie possible |
| visitedCount == len(visibleMap) | Always equal after every turn |

**Proceed to Phase 8 only when all checks pass.**
