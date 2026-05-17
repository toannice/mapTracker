# Test Guide: Phase 5 — User Story 3 (Collect Items and Use Clues)

**Phase goal**: All special cell types trigger correct effects on step: reward tile (3 random effects, repositions), trap tile (5 random penalties), teleport portal (exits partner), compass clue (private event only).

**Key principle**: Clue and penalty events must go ONLY to the affected player — never to all players.

---

## Prerequisites

- Phase 4 complete and passing
- Server running: `MAP_SIZE=5 TURN_SECONDS=30 LOG_LEVEL=debug go run ./cmd/server`
- Two wscat terminals (Alice + Bob) with an active game

---

## Strategy: Finding Special Cells

With a 5×5 map, special cells are sparse. The fastest approach:

1. Start the server with `LOG_LEVEL=debug`
2. Watch server logs when the game starts — debug logs should print the generated map layout
3. Add a temporary debug log line in `GenerateMap` that prints all special cell positions to stdout

Alternatively, navigate to every cell until you hit a special one (25 cells max on 5×5).

---

## Bullet Tile — Infinite Pickup (Regression from Phase 4)

Already tested in Phase 4. Confirm it still works after Phase 5 code changes.

Pick up bullet twice from same tile:
- First pickup: `inventory` gains `bullet`
- Second pickup: `inventory` gains another `bullet` (tile has infinite supply)
- Tile kind in `visibleMap` remains `"bullet"` both times

### Wrong if

Tile changes to `"empty"` after first pickup → regression introduced in Phase 5; tile kind must not be mutated on pickup.

---

## Reward Tile

### How to test

Move the active player onto a reward tile (kind = "reward"). The reward triggers automatically on step (no pickup action needed).

### Expected — turn_result contains an event

One of these three possible outcomes in `events`:

**Option A — All positions revealed**:
```json
{
  "kind": "reward_activated",
  "payload": {"effect": "all_positions_revealed"}
}
```
Both Alice and Bob receive this event. Both receive `others` list updated with all player positions.

**Option B — Nearest player direction (private)**:
```json
{
  "kind": "reward_activated",
  "payload": {"effect": "nearest_direction", "value": "NE"}
}
```
Only the player who stepped on the tile receives this. Bob must NOT see it.

**Option C — All bullet locations (private)**:
```json
{
  "kind": "reward_activated",
  "payload": {"effect": "all_bullet_locations"}
}
```
Only the stepping player receives the bullet positions.

### Reward Tile Repositions

After activation, the reward tile must move to a different random empty cell. In the next `visibleMap` of any player who has visited the old cell, the old position should now show as `"empty"`. If a player later visits the new position, it shows as `"reward"`.

### Wrong if

- No event in `turn_result` after stepping on reward → `resolveReward` not called
- Always produces the same effect → RNG not seeded; run game 5 times and expect different effects
- Reward tile stays in place → tile not repositioned after activation
- "nearest_direction" value is `""` (empty) → calculation missing or no other player exists
- Private reward effect sent to ALL players → broadcast instead of targeted send

---

## Trap Tile

### How to test

Move the active player onto a trap tile (kind = "trap"). Trap triggers automatically on step.

### Expected — one of five effects in `events`

**reveal_position**:
```json
{"kind": "trap_triggered", "payload": {"effect": "reveal_position"}}
```
ALL players receive this event. An additional event or updated others list reveals the trapped player's position to everyone.

**random_teleport**:
```json
{"kind": "trap_triggered", "payload": {"effect": "random_teleport"}}
```
Only trapped player receives this. Player's `self.pos` in the same `turn_result` is a different position (the teleport destination). Teleport must NOT land on another trap (re-trigger not allowed per spec edge case).

**lose_next_turn**:
```json
{"kind": "trap_triggered", "payload": {"effect": "lose_next_turn"}}
```
Only trapped player receives this. On the next turn cycle, when it's this player's turn, the turn is automatically skipped with reason `"trap"` and they receive a `turn_skipped` event.

**lose_bullet**:
```json
{"kind": "trap_triggered", "payload": {"effect": "lose_bullet"}}
```
Only trapped player receives this. The trapped player's `self.inventory` in the same `turn_result` has one fewer bullet.

**info_blackout**:
```json
{"kind": "trap_triggered", "payload": {"effect": "info_blackout"}}
```
Only trapped player receives this. On the player's next turn, any clue events (from compass or reward) are suppressed — `events` arrives empty even if a clue would have been generated. The `infoBlackout` flag in `self` shows `true` for that one turn, then resets.

### Wrong if

- No event in turn_result after stepping on trap → `resolveTrap` not called in `applyMove`
- Only one trap effect ever fires → RNG always returns 0; run 10 games and expect variation
- `lose_next_turn` causes immediate skip on THIS turn → flag applies to the NEXT turn, not current
- `info_blackout` persists across multiple turns → flag must be cleared after one turn
- Trap effect sent to all players when it should be private → check event scoping per effect type

---

## Trap: lose_next_turn — Sequence Test

1. Player A steps on trap, gets `lose_next_turn`
2. Player B takes a turn (normal)
3. Player A's turn arrives: expect `turn_skipped` event for A, turn advances to B again immediately
4. Player A's NEXT turn (3rd cycle): A can act normally

### Wrong if

- Player A can still act on the next turn → `SkipNextTurn` flag not applied or not checked
- Player A is skipped for TWO turns → flag not cleared after first skip

---

## Teleport Portal

### How to test

The map has an even number of portal pairs (portal_a / portal_b). Move the active player onto a portal_a cell.

### Expected

Player's `self.pos` in the `turn_result` is the position of the partner `portal_b` cell (NOT the portal_a cell they stepped into).

```json
{
  "kind": "portal_used",
  "payload": {}
}
```
Event is in `events` (optional — for UI feedback).

### Wrong if

- Player stays at portal_a position → teleport not applied; check `ResolvePortal` is called
- Player teleports to a position that is NOT the partner portal → PortalID pairing wrong; both cells must share the same `PortalID`
- Portal re-triggers immediately upon exit → `portal_b` exit must not re-trigger `portal_a`'s handler

---

## Compass Item — Private Clue

### Setup

Ensure a player has a compass item in inventory. (Compass is a held item picked up via Pickup action on a compass cell, not a tile effect — test this separately from reward tiles.)

Move player onto a compass-type cell, then use Pickup:
```json
{"type":"action","ts":0,"data":{"kind":"pickup"}}
```

### Expected

```json
{
  "self": {
    "inventory": [{"id": "item_xxx", "kind": "compass"}]
  }
}
```

To use the compass (Phase 2 feature — `use_item` action):
```json
{"type":"action","ts":0,"data":{"kind":"use_item","itemId":"item_xxx"}}
```

### Expected — only the acting player receives

```json
{
  "type": "event",
  "data": {
    "kind": "clue_received",
    "payload": {
      "clueType": "nearest_direction",
      "value": "NE"
    }
  }
}
```

One of four possible clue types:
- `"nearest_direction"` with value like `"N"`, `"NE"`, `"SW"`, etc.
- `"own_start_pos"` with value `{"x":3,"y":1}`
- `"one_other_player_pos"` with value `{"playerName":"Bob","pos":{"x":2,"y":0}}`
- `"3x3_surroundings"` with value array of CellView objects

### Wrong if

- Both Alice and Bob receive the `clue_received` event → critical bug; clue must be private
- `clueType` is always the same → RNG not varying; test across 5 uses
- `value` is null or empty → clue calculation not implemented

---

## Info Blackout — Clue Suppression Test

1. Alice steps on info_blackout trap
2. Alice's next turn: Alice picks up compass and uses it
3. Verify: compass would produce a clue, but Alice receives NO `clue_received` event in that turn_result
4. Alice's turn after that: compass clue works normally

### Wrong if

- Alice receives clue despite info_blackout → `InfoBlackout` flag not suppressing events in `BuildPlayerView`
- Alice loses clue item without using it → info_blackout only suppresses delivery, not item consumption

---

## Random Teleport — No Re-trigger on Trap Destination

1. Place player via teleport onto a cell that is also a trap tile
2. Verify: trap at destination does NOT trigger immediately
3. Only when the player MOVES onto a trap cell normally does it trigger

Per spec edge case: "What happens if a random teleport lands a player on a trap tile?" → trap on destination must NOT re-trigger.

### Wrong if

- Trap fires again immediately after teleport → recursive effect resolver; apply destination cell effects only for intentional moves

---

## Phase 5 Pass Criteria

| Test | Expected |
|---|---|
| Reward tile: event fires | One of 3 effects in turn_result events |
| Reward tile: repositions | Old cell shows "empty" in future visits |
| Reward public effects | all_positions_revealed sent to all |
| Reward private effects | nearest_direction / all_bullet_locations sent only to stepping player |
| Trap tile: event fires | One of 5 effects in turn_result events |
| lose_next_turn | Exactly next turn skipped, then normal |
| info_blackout | Clues suppressed for exactly one turn |
| Teleport portal | Player exits at partner portal position |
| Trap at teleport destination | No re-trigger |
| Compass clue | Only acting player receives clue_received |
| Clue type varies | Different clue types across 5 uses |

**Proceed to Phase 6 only when all checks pass.**
