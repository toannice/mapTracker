# Research: Blind Map Survival

**Phase**: 0 — Decisions & Rationale
**Date**: 2026-05-16
**Source of truth**: `blind_map_survival_tech_spec.docx`

---

## D-001: Map Dimensions

**Decision**: Default map size 20×20 (400 cells); configurable via `MAP_SIZE` env var.
**Rationale**: 20×20 provides enough space for 8 players to spread out while keeping memory trivial (400 cells × ~50 bytes ≈ 20KB per room). The spec mentions "20×20 for example". Configurable for balance tuning without code changes.
**Alternatives considered**: 15×15 (too cramped for 8 players), 25×25 (larger but wasteful on free tier).

---

## D-002: Goroutine Channel Buffer Sizes

**Decision**: All room channels use buffer size 32.
**Rationale**: A buffer of 32 prevents backpressure on burst inputs (e.g., all 8 players submitting actions within milliseconds). At 8 players/room and 1 action/turn, steady-state throughput is trivially low. 32 provides comfortable headroom without wasting memory (~256 bytes/channel at 32-pointer depth).
**Alternatives considered**: Unbuffered (risk of goroutine blocking under burst), 256 (unnecessary memory on free tier).

---

## D-003: RNG Strategy for Map Generation

**Decision**: Use `crypto/rand` to seed `math/rand/v2` once per room at creation time. All subsequent random choices (item placement, trap effects, teleport pairing) use that seeded PRNG.
**Rationale**: `crypto/rand` seeding ensures map layouts are not reproducible by clients (they cannot predict the seed). `math/rand/v2` provides fast pseudo-random generation for in-room use. Spec explicitly requires `crypto/rand` seed.
**Alternatives considered**: Pure `math/rand` with time seed (predictable), full `crypto/rand` for every draw (too slow for game loop).

---

## D-004: Turn Deadline Enforcement

**Decision**: Room goroutine runs a `time.NewTicker(100ms)`. On each tick, it checks `time.Now().After(state.TurnDeadline)`. If true, `advanceTurn()` is called.
**Rationale**: 100ms granularity means a player's turn can expire up to 100ms late at worst — imperceptible for a 30-second turn. The ticker is reused for the room lifetime; it is stopped on room shutdown. This is event-driven: game logic only runs when an action arrives or the ticker fires.
**Alternatives considered**: `time.After` per-turn (creates a new channel each turn, minor GC pressure), `time.Sleep` loop (wastes CPU), dedicated timer goroutine (unnecessary complexity).

---

## D-005: WebSocket Library

**Decision**: `github.com/coder/websocket` v1.8+ (formerly `nhooyr.io/websocket`).
**Rationale**: gorilla/websocket is archived (no longer maintained). coder/websocket is the actively maintained successor, integrates cleanly with `context.Context`, and supports zero-dependency usage. Spec explicitly requires this library.
**Alternatives considered**: gorilla/websocket (archived), gobwas/ws (lower-level, no context support).

---

## D-006: Kotlin Multiplatform Project Structure

**Decision**: Gradle multi-project: `client/settings.gradle.kts` declares `shared`, `androidApp`, `desktopApp` subprojects. The `shared` module uses `commonMain` source set for all platform-agnostic logic.
**Rationale**: KMP 2.0+ uses the new `kotlin-multiplatform` plugin with `jvm()`, `androidTarget()` targets declared in `shared/build.gradle.kts`. This is the canonical KMP 2.0 project layout.
**Alternatives considered**: Single-module KMP (harder to add new targets), separate repos (too much overhead for solo/small team).

---

## D-007: Ktor WebSocket Reconnect Pattern

**Decision**: `ReconnectManager` owns the reconnect loop: catches WS close → waits `1s × 2^attempt` (cap 30s) → re-calls `WebSocketClient.connect()` → on success sends `{type:"rejoin", data:{playerId:"..."}}`. Max 5 attempts before terminal failure.
**Rationale**: Exponential backoff prevents hammering the server during cold-start (up to 30s wake time on Render free tier). Rejoin uses the old `playerId` so the server can restore state. Spec mandates this exact backoff sequence.
**Alternatives considered**: Fixed-interval retry (hammers server on cold start), immediate retry (burns CPU/battery on mobile).

---

## D-008: PlayerView Filtering Strategy

**Decision**: Server maintains one `GameState` with full position data. On each `turn_result` broadcast, the server generates a unique `PlayerView` per connected client — serialising only the fields that player is permitted to see.
**Rationale**: This is the authoritative server model: a single source of truth, filtered at the serialisation boundary. No client ever receives another player's position, health, or inventory unless an item effect explicitly grants that information.
**Alternatives considered**: Separate per-player state (complex synchronisation), client-side filtering (client must be trusted — unacceptable).

---

## D-009: Map Submission Win Condition Tracking

**Decision**: `Player` struct includes `VisitedCells map[Position]bool`. On each move, the room appends the new position to this set. On `Submit Map` action, the server checks `len(player.VisitedCells) == mapSize*mapSize`.
**Rationale**: Simple O(1) amortised tracking per move. The full-map check is O(1) since visited count is maintained incrementally. Server is authoritative — client cannot fake visited cells.
**Alternatives considered**: Bitmask (more compact but less readable for MVP), client-reported list (cannot trust client).

---

## D-010: Render.com Cold Start Mitigation

**Decision**: Client shows "Waking server…" progress indicator immediately on connect. WebSocket connect has 60s timeout. `/healthz` responds immediately (< 100ms) without touching game state.
**Rationale**: Render free tier sleeps after 15min idle; wake takes ~30s. 60s gives comfortable headroom. The `/healthz` endpoint is intentionally minimal to avoid false-positive health failures during startup.
**Alternatives considered**: UptimeRobot ping every 10min (grey-area, not needed for MVP), Fly.io (discontinued free tier).
