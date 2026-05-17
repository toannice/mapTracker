package protocol

import (
	"github.com/your-org/blindmap/internal/game"
)

type SelfView struct {
	ID           game.PlayerID `json:"id"`
	Name         string        `json:"name"`
	Pos          game.Position `json:"pos"`
	Alive        bool          `json:"alive"`
	Inventory    []game.Item   `json:"inventory"`
	VisitedCount int           `json:"visitedCount"`
	TotalCells   int           `json:"totalCells"`
	InfoBlackout bool          `json:"infoBlackout"`
}

type OtherPlayerView struct {
	ID    game.PlayerID `json:"id"`
	Name  string        `json:"name"`
	Alive bool          `json:"alive"`
}

type CellView struct {
	Pos  game.Position `json:"pos"`
	Kind game.CellKind `json:"kind"`
}

type PlayerView struct {
	Self        SelfView          `json:"self"`
	Others      []OtherPlayerView `json:"others"`
	VisibleMap  []CellView        `json:"visibleMap"`
	Events      []Event           `json:"events"`
	TurnEndsAt  int64             `json:"turnEndsAt"`
	CurrentTurn game.PlayerID     `json:"currentTurn"`
	Turn        int               `json:"turn"`
	Phase       game.Phase        `json:"phase"`
}

func BuildPlayerView(state *game.GameState, playerID game.PlayerID, events []Event) PlayerView {
	p := state.Players[playerID]

	self := SelfView{
		ID:           p.ID,
		Name:         p.Name,
		Pos:          p.Pos,
		Alive:        p.Alive,
		Inventory:    p.Inventory,
		VisitedCount: len(p.VisitedCells),
		TotalCells:   state.MapSize * state.MapSize,
		InfoBlackout: p.InfoBlackout,
	}
	if self.Inventory == nil {
		self.Inventory = []game.Item{}
	}

	others := make([]OtherPlayerView, 0, len(state.Players)-1)
	for id, other := range state.Players {
		if id != playerID {
			others = append(others, OtherPlayerView{
				ID:    other.ID,
				Name:  other.Name,
				Alive: other.Alive,
			})
		}
	}

	visible := make([]CellView, 0, len(p.VisitedCells))
	for pos := range p.VisitedCells {
		cell := state.Grid[pos.Y][pos.X]
		visible = append(visible, CellView{Pos: cell.Pos, Kind: cell.Kind})
	}

	// suppress clue events when InfoBlackout is active, then clear it
	filteredEvents := events
	if p.InfoBlackout {
		filteredEvents = make([]Event, 0)
		for _, e := range events {
			if e.Kind != EventClueReceived {
				filteredEvents = append(filteredEvents, e)
			}
		}
		p.InfoBlackout = false
	}

	var currentTurn game.PlayerID
	if len(state.TurnOrder) > 0 {
		currentTurn = state.TurnOrder[state.CurrentIdx%len(state.TurnOrder)]
	}

	return PlayerView{
		Self:        self,
		Others:      others,
		VisibleMap:  visible,
		Events:      filteredEvents,
		TurnEndsAt:  state.TurnDeadline.UnixMilli(),
		CurrentTurn: currentTurn,
		Turn:        state.Turn,
		Phase:       state.Phase,
	}
}

