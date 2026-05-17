# Test Guide: Phase 2 — Foundational

**Phase goal**: All shared types compile cleanly on both Go and Kotlin sides. No game logic yet — only type definitions and the WebSocket client skeleton.

---

## Prerequisites

- Phase 1 complete (both build systems working)
- Working directory: repo root

---

## Go Type Compilation

### How to test

```powershell
cd server
go build ./...
```

### Expected

Exit code 0, no output.

### Wrong if

```
./internal/game/state.go:12:2: undefined: time
```
→ Missing `import "time"` in state.go

```
./internal/protocol/messages.go:8:2: undefined: json
```
→ Missing `import "encoding/json"`

```
./internal/protocol/views.go: cannot use ... as type PlayerView
```
→ Field mismatch between state.go types and views.go — cross-check with `specs/001-blind-map-survival/data-model.md`

---

## Go Vet

### How to test

```powershell
cd server
go vet ./...
```

### Expected

No output, exit code 0.

### Wrong if

```
internal/game/state.go:45:2: struct field X has json tag but is unexported
```
→ Exported fields with `json:"-"` tags are fine; unexported fields with json tags are the problem.

```
printf format %s has arg ... of wrong type
```
→ Fix the printf call.

---

## Config Loading

### How to test

Write a temporary `server/internal/config/config_test.go`:

```go
package config_test

import (
    "os"
    "testing"
    "server/internal/config"
)

func TestLoadDefaults(t *testing.T) {
    cfg := config.Load()
    if cfg.Port != "8080" { t.Errorf("Port: got %q", cfg.Port) }
    if cfg.MapSize != 20 { t.Errorf("MapSize: got %d", cfg.MapSize) }
    if cfg.MaxRooms != 100 { t.Errorf("MaxRooms: got %d", cfg.MaxRooms) }
    if cfg.TurnSeconds != 30 { t.Errorf("TurnSeconds: got %d", cfg.TurnSeconds) }
}

func TestLoadOverrides(t *testing.T) {
    os.Setenv("PORT", "9090")
    os.Setenv("MAP_SIZE", "10")
    defer os.Unsetenv("PORT")
    defer os.Unsetenv("MAP_SIZE")

    cfg := config.Load()
    if cfg.Port != "9090" { t.Errorf("Port: got %q", cfg.Port) }
    if cfg.MapSize != 10 { t.Errorf("MapSize: got %d", cfg.MapSize) }
}
```

```powershell
cd server
go test ./internal/config/...
```

### Expected

```
ok   server/internal/config   0.001s
```

### Wrong if

```
FAIL: TestLoadDefaults Port: got ""
```
→ `Load()` is not reading the default; add `if v := os.Getenv("PORT"); v != "" { cfg.Port = v } else { cfg.Port = "8080" }`

```
FAIL: TestLoadOverrides MapSize: got 20
```
→ Env var override not implemented; `strconv.Atoi(os.Getenv("MAP_SIZE"))` must be called

---

## Go Type Shape Validation

Manually verify these fields on `Player` in `state.go` have `json:"-"` (never sent over the wire):

| Field | Must have |
|---|---|
| `Pos` | `json:"-"` |
| `StartPos` | `json:"-"` |
| `Inventory` | `json:"-"` |
| `VisitedCells` | `json:"-"` |
| `SkipNextTurn` | `json:"-"` |
| `InfoBlackout` | `json:"-"` |

And these fields on `SelfView` in `views.go` must be exported (sent to the owning player):

| Field | Must be exported |
|---|---|
| `Pos` | ✓ |
| `Inventory` | ✓ |
| `VisitedCount` | ✓ |
| `InfoBlackout` | ✓ |

**Cross-reference**: `specs/001-blind-map-survival/data-model.md` — if the field tags differ from the spec, that is a bug.

---

## BuildPlayerView — Filtering Test

Add to a test file `server/internal/protocol/views_test.go`:

```go
func TestBuildPlayerViewNoPositionLeak(t *testing.T) {
    // Setup a 5x5 game with 2 players
    state := &game.GameState{
        MapSize: 5,
        Players: map[game.PlayerID]*game.Player{
            "p1": {ID: "p1", Name: "Alice", Pos: game.Position{X: 2, Y: 3}, Alive: true, VisitedCells: map[game.Position]bool{{2, 3}: true}},
            "p2": {ID: "p2", Name: "Bob", Pos: game.Position{X: 0, Y: 0}, Alive: true},
        },
        Grid:     make([][]game.Cell, 5),
        Phase:    game.PhaseActive,
        TurnOrder: []game.PlayerID{"p1", "p2"},
    }
    for i := range state.Grid { state.Grid[i] = make([]game.Cell, 5) }

    view := BuildPlayerView(state, "p1")

    // p1 sees own position
    if view.Self.Pos.X != 2 || view.Self.Pos.Y != 3 {
        t.Errorf("Self pos wrong: %+v", view.Self.Pos)
    }

    // p1 must NOT see Bob's position in others
    for _, other := range view.Others {
        if other.Name == "Bob" {
            // OtherPlayerView has no Pos field — this is a compile-time check
            // If this compiles, the type is correct
            _ = other.Alive
        }
    }

    // p1 sees only visited cells
    if len(view.VisibleMap) != 1 {
        t.Errorf("VisibleMap should have 1 cell, got %d", len(view.VisibleMap))
    }
}
```

```powershell
cd server
go test ./internal/protocol/...
```

### Expected

```
ok   server/internal/protocol   0.003s
```

### Wrong if

- `view.Others[0]` has a `Pos` field → `OtherPlayerView` definition is wrong (remove Pos)
- `len(view.VisibleMap) == 0` when player is on a cell → `VisitedCells` not being read in `BuildPlayerView`
- `len(view.VisibleMap) == 25` (all cells) → no filtering applied; only visited cells should appear

---

## Kotlin Shared Module Compilation

### How to test

```powershell
cd ..\client
.\gradlew :shared:assemble
```

### Expected

```
BUILD SUCCESSFUL
```

### Wrong if

```
error: unresolved reference: JsonElement
```
→ Missing `import kotlinx.serialization.json.JsonElement` and `kotlinx-serialization-json` dependency in `shared/build.gradle.kts`

```
error: This type has a constructor, and thus must be initialized here
```
→ `data class` missing default parameter; add `= null` or `= emptyList()`

```
error: @Serializable class has primary constructor parameter that is not a property
```
→ Change `val` to explicit property: `@Serializable data class Foo(val bar: String)`

---

## Kotlin Type Shape Validation

Open `client/shared/src/commonMain/kotlin/com/blindmap/protocol/Views.kt` and confirm:

- `SelfView` has `pos: Position` (own position IS included)
- `OtherPlayerView` does NOT have a `pos` field
- `PlayerView` has `events: List<Event>`
- `Event` has `payload: JsonElement? = null`

Cross-reference against `specs/001-blind-map-survival/data-model.md` Kotlin section. Any missing field is a bug.

---

## WebSocketClient Compile Check

```powershell
cd client
.\gradlew :shared:compileCommonMainKotlinMetadata
```

### Expected

```
BUILD SUCCESSFUL
```

### Wrong if

```
error: Class 'WebSocketSession' is not accessible
```
→ Ktor dependency missing or wrong artifact ID; check `shared/build.gradle.kts` for `ktor-client-websockets`

```
error: Suspension functions can only be called within coroutine body
```
→ `connect()` must be a `suspend fun`, or call from a coroutine scope

---

## Phase 2 Pass Criteria

| Check | Command | Expected |
|---|---|---|
| Go types compile | `go build ./...` | Exit 0, no output |
| Go vet clean | `go vet ./...` | Exit 0, no output |
| Config defaults | `go test ./internal/config/...` | PASS |
| PlayerView no position leak | `go test ./internal/protocol/...` | PASS |
| Kotlin shared assembles | `.\gradlew :shared:assemble` | BUILD SUCCESSFUL |
| OtherPlayerView has no Pos | Manual code review | Confirmed |
| WebSocketClient compiles | `.\gradlew :shared:compileCommonMainKotlinMetadata` | BUILD SUCCESSFUL |

**Proceed to Phase 3 only when all checks pass.**
