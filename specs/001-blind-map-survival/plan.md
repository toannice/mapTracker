# Implementation Plan: Blind Map Survival

**Branch**: `001-blind-map-survival` | **Date**: 2026-05-16 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/001-blind-map-survival/spec.md`

## Summary

Build a multiplayer turn-based survival & deduction game where 2–8 players navigate a shared blind map via WebSocket, taking sequential turns with a 30-second timer. The server is authoritative (Go, goroutine-per-room), the client is cross-platform (Kotlin Multiplatform: Android Compose for Phase 1, Desktop TUI for Phase 2), and the entire stack deploys free on Render.com Singapore via Docker. Full architecture, tech stack, and deployment strategy are locked in `blind_map_survival_tech_spec.docx`.

## Technical Context

**Language/Version**: Go 1.23+ (backend) · Kotlin 2.0+ / KMP (client)
**Primary Dependencies**:
- Backend: `github.com/coder/websocket` v1.8+, Go standard library (`net/http`, `encoding/json`, `log/slog`, `crypto/rand`)
- Client shared: Ktor Client 3.0+ (WebSocket), `kotlinx.serialization`, Kotlin Coroutines + Flow
- Android: Jetpack Compose 1.7+
- Desktop (Phase 2): Mordant 2.7+

**Storage**: None at MVP — all state in-memory; restart = games lost (accepted trade-off)
**Testing**: `go test -race ./...` (backend) · `./gradlew :shared:test` (KMP)
**Target Platform**: Linux server (Render.com) · Android 8+ · Linux/Windows terminal (Phase 2)
**Project Type**: Multiplayer game server + cross-platform client
**Performance Goals**: Action → state update < 200ms; turn resolution P99 < 50ms; connection setup (warm) < 500ms RTT VN→SG
**Constraints**: Free-tier 512MB RAM; ≤50 concurrent rooms; ≤400 concurrent connections; Docker image < 20MB
**Scale/Scope**: 2–8 players/room, up to 50 rooms, private rooms only (no matchmaking at MVP)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Principle | Gate Question | Status |
|---|-----------|---------------|--------|
| I | Code Cleanliness | Are all planned source files scoped to ≤500 lines? Split any file expected to exceed this before design is finalised. | ☑ |
| II | Spec-First Development | Does every planned component trace back to `blind_map_survival_tech_spec.docx`? Any deviation from the spec MUST be flagged for user approval before it enters the plan. | ☑ |
| III | Commit Discipline | Is the commit cadence plan clear? Every >100-line change batch MUST produce one commit with message format `add\|fix\|delete: <work>`. | ☑ |
| IV | Phase Test Gate | Does each implementation phase have a corresponding `test-phase-<N>.md` guide? All Pass Criteria in that guide MUST be verified against running behavior before the phase is marked done. | ☑ |

**Gate I rationale**: Source directory is pre-split into single-responsibility files; see Project Structure below — no planned file exceeds 400 lines.
**Gate II rationale**: All components derive directly from `blind_map_survival_tech_spec.docx` sections 2–7. No technology introduced outside the spec.
**Gate III rationale**: Commit cadence follows Principle III — each task group triggers a commit when accumulated changes exceed 100 lines.
**Gate IV rationale**: Nine `test-phase-N.md` guides exist in `specs/001-blind-map-survival/`, one per implementation phase, each with explicit Pass Criteria that must be verified against running behavior before proceeding.

*All four gates pass — proceeding to Phase 0.*

## Project Structure

### Documentation (this feature)

```text
specs/001-blind-map-survival/
├── plan.md              # This file
├── research.md          # Phase 0 decisions
├── data-model.md        # Phase 1 entity definitions (Go + Kotlin)
├── quickstart.md        # Phase 1 local run guide
├── contracts/
│   └── websocket-protocol.md   # WebSocket message contract
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
server/                          # Go backend
├── cmd/server/
│   └── main.go                  # HTTP server setup, graceful shutdown (~100 lines)
├── internal/
│   ├── config/
│   │   └── config.go            # Env var loading (~60 lines)
│   ├── hub/
│   │   └── hub.go               # Hub goroutine, room registry (~180 lines)
│   ├── room/
│   │   ├── room.go              # Room goroutine, select loop (~350 lines)
│   │   └── turn.go              # Turn resolution, action dispatch (~300 lines)
│   ├── game/
│   │   ├── state.go             # GameState, Player, Phase types (~200 lines)
│   │   ├── map.go               # Map generation, cell placement (~280 lines)
│   │   └── items.go             # Item/tile effect resolution (~250 lines)
│   ├── conn/
│   │   └── conn.go              # readPump + writePump goroutines (~150 lines)
│   └── protocol/
│       ├── messages.go          # Envelope, client→server types (~150 lines)
│       └── views.go             # PlayerView, filtered serialisation (~120 lines)
├── go.mod
├── go.sum
├── Dockerfile
└── render.yaml

client/                          # Kotlin Multiplatform
├── shared/src/commonMain/kotlin/com/blindmap/
│   ├── net/
│   │   ├── WebSocketClient.kt   # WS connect, send, receive (~200 lines)
│   │   └── ReconnectManager.kt  # Exponential backoff, rejoin (~150 lines)
│   ├── protocol/
│   │   ├── Messages.kt          # Envelope, action, event DTOs (~200 lines)
│   │   └── Views.kt             # ClientGameState, SelfView, OtherView (~150 lines)
│   ├── state/
│   │   ├── GameState.kt         # ClientGameState data class (~120 lines)
│   │   └── Reducer.kt           # Pure reducer: Event → GameState (~200 lines)
│   └── viewmodel/
│       └── GameViewModel.kt     # StateFlow exposure, action dispatch (~200 lines)
├── androidApp/src/main/kotlin/com/blindmap/android/
│   ├── MainActivity.kt          # Activity, nav setup (~80 lines)
│   └── ui/
│       ├── LobbyScreen.kt       # Room create/join, player list (~220 lines)
│       ├── GameScreen.kt        # Map grid, turn actions, events (~380 lines)
│       └── GameOverScreen.kt    # Winner display, stats (~120 lines)
├── desktopApp/ (Phase 2)
│   └── src/main/kotlin/com/blindmap/desktop/
│       ├── Main.kt              # JVM entry, Mordant init (~70 lines)
│       └── tui/
│           ├── GameTUI.kt       # Screen render, map display (~300 lines)
│           └── InputHandler.kt  # Key binding, action mapping (~150 lines)
├── build.gradle.kts             # KMP root build
└── settings.gradle.kts
```

**Structure Decision**: Multi-project layout (server/ + client/) at repository root. Backend is a standalone Go module; client is a KMP Gradle project with shared + androidApp + desktopApp subprojects. Both are independent — backend can be built and tested without the client.

## Complexity Tracking

> No Constitution Check violations — this section is intentionally empty.
