# Phase 2 — Blind Map Survival: UX Overhaul & Map-Reconstruction Mechanic

Status: **implemented** (2026-05-18) — server + client code complete; Android
build pending verification on a JDK 17 machine.
Branch: `001-blind-map-survival`

This document is the agreed direction for Phase 2. It supersedes the loose
"TODO-2 · UX overhaul" item in `CLAUDE.md`.

---

## 1. Design intent

Blind Map Survival is a **deduction** game. Phase 2 doubles down on that:

- The player is **never told where they are** — no coordinates, no exploration
  map in the main screen.
- Everything a player learns comes from a **narrative event feed** that reports
  every action of every player ("Toan moved left — hit a wall").
- The win-by-mapping condition becomes an explicit **memory reconstruction**:
  the player draws the wall layout from memory on a blank grid and submits it.

---

## 2. Server changes (Go)

### 2.1 Interior walls

- Add `CellWall` to `CellKind` (`internal/game/state.go`).
- `GenerateMap` (`internal/game/map.go`): place walls at **~25% of cells**.
- **Connectivity is mandatory**: after placing walls, run a flood-fill from any
  non-wall cell. If any non-wall cell is unreachable, regenerate the map.
  Without this check, 25% density can produce isolated regions where a player
  is trapped.
- Player spawns and `bullet` / `reward` / `trap` / `portal` placement must
  avoid `CellWall`.

### 2.2 `player_moved` event (broadcast to everyone)

Every move — success or failure — emits one event sent to **all** players.

Payload: `{ playerName, direction, success, blockType }`
- **No `pos` field.** Coordinates are never exposed for normal moves.
- `success = false` → the move hit a wall (map border *or* interior wall — the
  event does not distinguish; others must deduce).
- `blockType` (only when `success = true`): one of
  `blank | bullet | reward | trap | portal`.
- `direction`: `up | down | left | right` (see §4).

### 2.3 Hitting a wall costs the turn

`applyMove` no longer returns an error for out-of-bounds / wall cells. Instead
it emits `player_moved` with `success = false` and the turn **still advances**.
Hitting a wall = a wasted turn.

### 2.4 Remove pickup

- Delete the `pickup` action and `applyPickup`.
- Moving onto a `bullet` cell auto-grants a bullet, **capped at 1**. If the
  player already holds a bullet, the move still reports the `bullet` blockType
  but no bullet is added (event text notes "already full").

### 2.5 Map cell-count stats (for the Info button)

- The server already knows the exact count of each `CellKind` after
  `GenerateMap`.
- Expose these counts to clients in the `game_start` view (and `welcome` /
  reconnect views) as a `mapStats` object:
  `{ mapSize, counts: { wall, blank, trap, reward, bullet, portal } }`.
- This is **aggregate** information only — no positions — so it does not violate
  the wire-security rule. It is shared knowledge for all players and is the data
  source for the client Info button (§3.6).

### 2.6 Submit map

- New action `submit_map`. Payload = the set of cells the player marked as
  walls on the blank grid.
- **Does not cost a turn** — allowed any time while the game is `active`,
  outside the normal turn flow.
- Server compares the submitted wall set against the real wall set:
  - **Exact match** → that player wins (`WinReason: "map_complete"`).
  - **Mismatch** → `player.maxSubmit -= 1`; broadcast `map_submitted` event to
    everyone: *"A submitted the map — wrong X cells"*, where `X` is the size of
    the symmetric difference (cells wrongly marked as wall + real walls missed).
- `maxSubmit` initial value = **3**. At 0, the player loses the right to submit
  and can only win by `last_alive`.

---

## 3. Client changes (Android / KMP)

### 3.1 Main GameScreen

- Remove the exploration grid and all x-y coordinate display.
- Main screen = **keypad** (movement + shoot) + **rolling event feed**.

### 3.2 Accumulating event feed

- `ClientGameState` gains `eventLog: List<Event>`.
- The Reducer **appends** new events from each `turn_result` (does not replace);
  the server only sends the current turn's events, so the client owns history.
- Each new event shows as a **3–5 s pop-up notification**, then settles into the
  scrollable feed.

### 3.3 Narrative event sentences

Always use the **real player name**, including for the local player (never
"You"). Examples:

- `"Toan moved left — blank tile"`
- `"Toan moved left — hit a wall"`
- `"Toan moved up — found a bullet"` / `"Toan moved up — bullet tile (full)"`
- shoot / eliminated / teleport / trap / reward / map-submitted → their own
  sentences, all broadcast to everyone.

### 3.4 "Map" button — reconstruction canvas

- A **Map** button opens an MxN grid matching the real map size, all white
  (blank canvas — reveals nothing).
- Tap a cell → black (guessed wall). Tap again → white.
- A **Submit** button inside; shows remaining submit attempts.

### 3.5 "Info" button — map cell-count table

- Available once the match has started (alongside the Map button).
- Tapping **Info** opens a table built from the `mapStats` payload (§2.5):

  ```
  Map: 8x8

  Type    Count
  wall      20
  blank     46
  trap       3
  reward     4
  bullet     6
  portal     2
  ```

- Reveals only totals, never positions — it is a deduction aid (e.g. "20 walls
  total, I've found 12 — 8 left to locate").

### 3.6 Error separation

- `transientError` — from the server `error` envelope (`NOT_YOUR_TURN`,
  `NO_BULLET`, `MAP_INCOMPLETE`, etc.) → auto-dismiss after 2–3 s.
- `connectionError` — client-side network/reconnect failures → persistent
  banner, cleared only on successful reconnect.

---

## 4. Direction naming

Event logs and UI use **`up` / `down` / `left` / `right`**, not compass terms.

Server-internal mapping (`game.Direction` stays as-is on the wire if needed,
but event payloads use the friendly form):

| Internal | Friendly |
|----------|----------|
| N        | up       |
| S        | down     |
| E        | right    |
| W        | left     |

---

## 5. Unchanged

- Coordinate-revealing trap/reward effects (`reveal_position`,
  `all_positions_revealed`, `all_bullet_locations`) keep their x-y payloads —
  these are deliberate game mechanics, distinct from normal-move privacy.
- Winning by `last_alive` remains.
- Authoritative-server model, WebSocket envelope, room/hub architecture.
