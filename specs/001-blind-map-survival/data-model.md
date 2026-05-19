# Data Model: Blind Map Survival

**Phase**: 1 — Entity Definitions
**Date**: 2026-05-16

All types are in-memory only (no persistence at MVP). Server-side types are in Go; client-side types are in Kotlin.

---

## Server-Side (Go) — `internal/game/state.go`

```go
type PlayerID string
type RoomID  string

type Phase string
const (
    PhaseLobby  Phase = "lobby"
    PhaseActive Phase = "active"
    PhaseEnded  Phase = "ended"
)

type Direction string
const (
    DirN Direction = "N"
    DirS Direction = "S"
    DirE Direction = "E"
    DirW Direction = "W"
)

type Position struct {
    X int `json:"x"`
    Y int `json:"y"`
}

type ItemKind string
const (
    ItemBullet  ItemKind = "bullet"
    ItemCompass ItemKind = "compass"
)

type Item struct {
    ID   string   `json:"id"`
    Kind ItemKind `json:"kind"`
}

type Player struct {
    ID           PlayerID            `json:"id"`
    Name         string              `json:"name"`
    Pos          Position            `json:"-"`          // never sent to other clients
    StartPos     Position            `json:"-"`          // never sent to anyone
    Alive        bool                `json:"alive"`
    SkipNextTurn bool                `json:"-"`
    InfoBlackout bool                `json:"-"`          // trap effect: no clues next turn
    Inventory    []Item              `json:"-"`          // never sent to other clients
    VisitedCells map[Position]bool   `json:"-"`          // server tracks for Submit Map
    ConnectedAt  time.Time           `json:"-"`
    LastSeen     time.Time           `json:"-"`
}

type CellKind string
const (
    CellEmpty    CellKind = "empty"
    CellBullet   CellKind = "bullet"    // infinite pickups
    CellReward   CellKind = "reward"    // repositions after activation
    CellTrap     CellKind = "trap"      // random penalty on step
    CellPortalA  CellKind = "portal_a"  // part of a portal pair
    CellPortalB  CellKind = "portal_b"  // paired with portal_a
)

type Cell struct {
    Pos      Position `json:"pos"`
    Kind     CellKind `json:"kind"`
    PortalID int      `json:"portalId,omitempty"` // pairs cells with same PortalID
}

type GameState struct {
    RoomID       RoomID              `json:"roomId"`
    Turn         int                 `json:"turn"`
    Phase        Phase               `json:"phase"`
    MapSize      int                 `json:"mapSize"`       // square; default 20
    TurnOrder    []PlayerID          `json:"-"`             // ordered list
    CurrentIdx   int                 `json:"-"`             // index into TurnOrder
    Players      map[PlayerID]*Player `json:"-"`
    Grid         [][]Cell            `json:"-"`             // [y][x]
    TurnDeadline time.Time           `json:"-"`
    TurnSeconds  int                 `json:"turnSeconds"`   // default 30
    Winner       *PlayerID           `json:"winner,omitempty"`
}
```

---

## Server-Side (Go) — `internal/protocol/views.go`

Filtered views sent to each client. **Rule**: a client MUST NOT receive any field tagged `json:"-"` above.

```go
// SelfView is sent only to the player it describes
type SelfView struct {
    ID           PlayerID  `json:"id"`
    Name         string    `json:"name"`
    Pos          Position  `json:"pos"`          // own position IS included
    Alive        bool      `json:"alive"`
    Inventory    []Item    `json:"inventory"`    // own inventory IS included
    VisitedCount int       `json:"visitedCount"` // cells visited so far
    TotalCells   int       `json:"totalCells"`   // mapSize * mapSize
    InfoBlackout bool      `json:"infoBlackout"` // trap effect active
}

// OtherPlayerView sent for all OTHER players — position/inventory deliberately excluded
type OtherPlayerView struct {
    ID    PlayerID `json:"id"`
    Name  string   `json:"name"`
    Alive bool     `json:"alive"`
}

// CellView describes a cell the player has visited (or learned about via item)
type CellView struct {
    Pos  Position `json:"pos"`
    Kind CellKind `json:"kind"`
}

// PlayerView is the full per-turn state snapshot sent to one player
type PlayerView struct {
    Self        SelfView          `json:"self"`
    Others      []OtherPlayerView `json:"others"`
    VisibleMap  []CellView        `json:"visibleMap"`   // cells this player has visited
    Events      []Event           `json:"events"`       // events from the resolved turn
    TurnEndsAt  int64             `json:"turnEndsAt"`   // unix millis
    CurrentTurn PlayerID          `json:"currentTurn"`  // whose turn it is now
    Turn        int               `json:"turn"`
    Phase       Phase             `json:"phase"`
}
```

---

## Server-Side (Go) — `internal/protocol/messages.go`

```go
// Envelope wraps every message in both directions
type Envelope struct {
    Type string          `json:"type"`
    Ts   int64           `json:"ts"`   // epoch millis (server clock)
    Data json.RawMessage `json:"data"`
}

// --- Client → Server ---

type JoinData struct {
    RoomCode      string `json:"roomCode"`
    PlayerName    string `json:"playerName"`
    ClientVersion string `json:"clientVersion"`
    PlayerID      string `json:"playerId,omitempty"` // set on rejoin
}

type ActionKind string
const (
    ActionMove      ActionKind = "move"
    ActionPickup    ActionKind = "pickup"
    ActionShoot     ActionKind = "shoot"
    ActionSubmitMap ActionKind = "submit_map"
)

type ActionData struct {
    Kind      ActionKind `json:"kind"`
    Direction Direction  `json:"direction,omitempty"` // move, shoot
    ItemID    string     `json:"itemId,omitempty"`    // use_item (Phase 2)
}

// --- Server → Client ---

type EventKind string
const (
    EventTrapTriggered    EventKind = "trap_triggered"
    EventRewardActivated  EventKind = "reward_activated"
    EventPlayerEliminated EventKind = "player_eliminated"
    EventShotFired        EventKind = "shot_fired"
    EventMapSubmitted     EventKind = "map_submitted"
    EventPortalUsed       EventKind = "portal_used"
    EventClueReceived     EventKind = "clue_received"
    EventTurnSkipped      EventKind = "turn_skipped"
)

type Event struct {
    Kind    EventKind   `json:"kind"`
    Payload interface{} `json:"payload,omitempty"`
}

// WelcomeData is sent after successful join
type WelcomeData struct {
    PlayerID  PlayerID    `json:"playerId"`
    RoomState interface{} `json:"roomState"` // LobbyView
}

// LobbyView is the pre-game state
type LobbyView struct {
    RoomCode string   `json:"roomCode"`
    Players  []string `json:"players"` // player names only
    IsHost   bool     `json:"isHost"`
}

// GameOverData is the terminal event
type GameOverData struct {
    Winner    *string `json:"winner"` // nil = draw (all disconnected)
    WinReason string  `json:"winReason"` // "last_alive" | "map_complete"
}

// ErrorData is returned for invalid actions or server errors
type ErrorData struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}

// ServerShutdownData is sent before graceful restart
type ServerShutdownData struct {
    ReconnectAfterMs int `json:"reconnectAfterMs"`
}
```

---

## Client-Side (Kotlin) — `shared/commonMain`

### `protocol/Messages.kt`

```kotlin
@Serializable
data class Envelope(
    val type: String,
    val ts: Long,
    val data: JsonElement
)

@Serializable
data class JoinData(
    val roomCode: String,
    val playerName: String,
    val clientVersion: String,
    val playerId: String? = null   // set on rejoin
)

@Serializable
data class ActionData(
    val kind: String,              // "move" | "pickup" | "shoot" | "submit_map"
    val direction: String? = null, // "N" | "S" | "E" | "W"
    val itemId: String? = null
)
```

### `protocol/Views.kt`

```kotlin
@Serializable
data class Position(val x: Int, val y: Int)

@Serializable
data class Item(val id: String, val kind: String)

@Serializable
data class SelfView(
    val id: String,
    val name: String,
    val pos: Position,
    val alive: Boolean,
    val inventory: List<Item>,
    val visitedCount: Int,
    val totalCells: Int,
    val infoBlackout: Boolean
)

@Serializable
data class OtherPlayerView(
    val id: String,
    val name: String,
    val alive: Boolean
)

@Serializable
data class CellView(val pos: Position, val kind: String)

@Serializable
data class PlayerView(
    val self: SelfView,
    val others: List<OtherPlayerView>,
    val visibleMap: List<CellView>,
    val events: List<Event>,
    val turnEndsAt: Long,
    val currentTurn: String,
    val turn: Int,
    val phase: String
)

@Serializable
data class Event(val kind: String, val payload: JsonElement? = null)

@Serializable
data class LobbyView(
    val roomCode: String,
    val players: List<String>,
    val isHost: Boolean
)

@Serializable
data class WelcomeData(
    val playerId: String,
    val roomState: LobbyView
)

@Serializable
data class GameOverData(
    val winner: String?,
    val winReason: String
)
```

### `state/GameState.kt`

```kotlin
enum class ConnState { Connecting, Connected, Reconnecting, Failed }

enum class GamePhase { Lobby, Active, Ended }

data class ClientGameState(
    val roomId: String = "",
    val playerId: String = "",
    val phase: GamePhase = GamePhase.Lobby,
    val lobby: LobbyView? = null,
    val game: PlayerView? = null,
    val gameOver: GameOverData? = null,
    val connState: ConnState = ConnState.Connecting,
    val errorMessage: String? = null
)
```

---

## Entity Relationship Summary

```
Room (1) ──── owns ──── GameState (1)
GameState (1) ──── contains ──── Player (2–8)
GameState (1) ──── contains ──── Grid (MapSize × MapSize Cells)
Player (1) ──── holds ──── Item (0–N)
Player (1) ──── tracks ──── VisitedCells (Set<Position>)
Cell ──── may be ──── BulletTile | RewardTile | TrapTile | Portal | Empty
Portal ──── paired with ──── Portal (always 1:1 pair)
```

## State Lifecycle

```
Room: Created → Lobby → Active → Ended → Destroyed (after 60s)
Player: Joining → InLobby → Active → (Eliminated | Disconnected) → Observing
Turn: Waiting → ActionReceived | TimerExpired → Resolved → Next Player
```
