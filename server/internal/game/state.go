package game

import "time"

type PlayerID string
type RoomID string

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
	ID           PlayerID          `json:"id"`
	Name         string            `json:"name"`
	Pos          Position          `json:"-"`
	StartPos     Position          `json:"-"`
	Alive        bool              `json:"alive"`
	SkipNextTurn bool              `json:"-"`
	InfoBlackout bool              `json:"-"`
	MaxSubmit    int               `json:"-"`
	Inventory    []Item            `json:"-"`
	VisitedCells map[Position]bool `json:"-"`
	ConnectedAt  time.Time         `json:"-"`
	LastSeen     time.Time         `json:"-"`
}

// GameEvent is a game-layer event produced by action resolvers.
// The room layer converts these to protocol.Event for wire encoding.
type GameEvent struct {
	Kind    string
	Payload interface{}
}

type CellKind string

const (
	CellEmpty   CellKind = "empty"
	CellWall    CellKind = "wall"
	CellBullet  CellKind = "bullet"
	CellReward  CellKind = "reward"
	CellTrap    CellKind = "trap"
	CellPortalA CellKind = "portal_a"
	CellPortalB CellKind = "portal_b"
)

type Cell struct {
	Pos      Position `json:"pos"`
	Kind     CellKind `json:"kind"`
	PortalID int      `json:"portalId,omitempty"`
}

type GameState struct {
	RoomID           RoomID              `json:"roomId"`
	Turn             int                 `json:"turn"`
	Phase            Phase               `json:"phase"`
	MapSize          int                 `json:"mapSize"`
	TurnOrder        []PlayerID          `json:"-"`
	CurrentIdx       int                 `json:"-"`
	Players          map[PlayerID]*Player `json:"-"`
	Grid             [][]Cell            `json:"-"`
	TurnDeadline     time.Time           `json:"-"`
	TurnSeconds      int                 `json:"turnSeconds"`
	Winner           *PlayerID           `json:"winner,omitempty"`
	WinReason        string              `json:"winReason,omitempty"`
	Paused           bool                `json:"-"`
	PauseRemainingMs int64               `json:"-"`
}
