# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan at:
specs/001-blind-map-survival/plan.md

Supporting artifacts:
- specs/001-blind-map-survival/spec.md          (feature specification)
- specs/001-blind-map-survival/research.md      (key decisions & rationale)
- specs/001-blind-map-survival/data-model.md    (Go + Kotlin type definitions)
- specs/001-blind-map-survival/contracts/websocket-protocol.md  (WS message contract)
- specs/001-blind-map-survival/quickstart.md    (local run guide)
<!-- SPECKIT END -->

## Project

**Blind Map Survival** — a multiplayer turn-based survival & deduction game. Players share a map but cannot see each other's positions. Win by being the last alive or by mapping the entire board and submitting it.

Full technical specification: `blind_map_survival_tech_spec.docx` (extract with PowerShell ZipFile trick to read XML inside).

## Development Workflow (Spec Kit)

This repo uses **Spec Kit v0.8.4** — a specification-driven development workflow. All feature work follows this sequence:

```
/speckit-specify   → generate spec.md from description (creates feature branch)
/speckit-clarify   → resolve ambiguities in the spec
/speckit-plan      → generate implementation plan (plan.md)
/speckit-tasks     → generate ordered task list (tasks.md)
/speckit-implement → execute tasks
/speckit-analyze   → cross-artifact consistency check
```

Feature branches must follow naming: `001-feature-name`, `1234-feature-name`, or `20260319-143022-feature-name`. The git hooks in `.specify/extensions.yml` auto-commit at each stage (optional, prompted).

Spec artifacts live under `.specify/features/<branch-name>/`: `spec.md`, `plan.md`, `tasks.md`.

## Planned Tech Stack

| Layer | Choice |
|---|---|
| Backend | Go 1.23+, `net/http` + `github.com/coder/websocket` |
| Client shared | Kotlin Multiplatform 2.0+ (`commonMain`) |
| Android UI | Jetpack Compose 1.7+ |
| Desktop UI | Mordant TUI 2.7+ (Linux/Windows terminal) |
| Client network | Ktor Client 3.0+ (WebSocket) |
| Serialization | `kotlinx.serialization` (client) / `encoding/json` (server) |
| Container | Docker multi-stage, `distroless/static-debian12:nonroot` final image |
| Hosting | Render.com Singapore, free Web Service tier |

## Architecture

**Authoritative server model**: all game state lives server-side; clients are thin renderers.

### Backend (Go)
- `cmd/server/` — entry point
- `internal/hub/` — central goroutine routing connections to rooms; owns `map[RoomID]*Room` behind `sync.RWMutex`
- `internal/room/` — one goroutine per active game; single-writer ownership of `GameState`; receives events via channels (`join`, `leave`, `actions`, `broadcast`)
- `internal/conn/` — each WebSocket connection has `readPump` + `writePump` goroutines
- `internal/protocol/` — JSON message envelope `{type, ts, data}`; DTOs; server→client filtered `PlayerView`

Key invariant: **all `GameState` mutations happen inside the Room goroutine** — no locks needed on game data structs.

### Client (Kotlin Multiplatform)
- `shared/commonMain/net/` — WebSocket client, reconnect with exponential backoff (1s→30s, max 5 retries)
- `shared/commonMain/protocol/` — DTOs matching server wire format
- `shared/commonMain/state/` — `ClientGameState`, reducer-style updates
- `shared/commonMain/viewmodel/` — platform-agnostic presentation logic, exposes `StateFlow`
- `androidApp/` — Compose UI, Activity, lifecycle
- `desktopApp/` — JVM `main()`, Mordant TUI, keyboard input

State flow is strictly unidirectional: WebSocket → Reducer → `ClientGameState` → ViewModel → UI.

## Build Commands

### Backend (Go)
```bash
# Run server locally
go run ./cmd/server

# Build production binary
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o out/server ./cmd/server

# Run tests (with race detector)
go test -race -timeout 60s ./...

# Vet
go vet ./...

# Build Docker image
docker build -t blind-map-survival .
```

### Client (Kotlin/Gradle)
```bash
# Android debug APK
./gradlew :androidApp:assembleDebug

# Desktop JAR (JVM)
./gradlew :desktopApp:run

# Run all tests
./gradlew test

# KMP shared module tests only
./gradlew :shared:test
```

## Protocol

All WebSocket messages use envelope: `{"type": "<msg_type>", "ts": <epoch_ms>, "data": {...}}`

Client→Server: `join`, `action` (kind: `move`/`use_item`/`pickup`), `ping`
Server→Client: `welcome`, `lobby_update`, `game_start`, `turn_result`, `event`, `game_over`, `error`, `pong`, `server_shutdown`

**Wire security rule**: `PlayerView` sent to client X **never** includes position, health, or inventory of other players. Server never trusts client-sent state.

## Environment Variables (Backend)

| Var | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `LOG_LEVEL` | `info` | `debug`/`info`/`warn`/`error` |
| `MAX_ROOMS` | `100` | Concurrent room limit |
| `TURN_SECONDS` | `30` | Default turn duration |
| `ALLOWED_ORIGINS` | `*` | CORS/CSWSH origin whitelist |

## Key Design Decisions

- **No DB at MVP**: all state in-memory; server restart = games lost (acceptable, matches are 5–15 min)
- **Sequential turns**: players act one at a time (a→b→c→d), not simultaneously
- **No body blocking**: players can occupy the same cell with no collision effect
- **Private rooms only**: 6-char alphanumeric room codes, no matchmaking at MVP
- **Cold start**: Render free tier sleeps after 15 min idle; client shows progress bar, retries up to 60s
- **Map logging for debug**: server logs full map state for replay/audit; no active anti-cheat needed beyond authoritative server
- **RNG seeded with `crypto/rand`**: map generation and item randomization are not reproducible by clients
