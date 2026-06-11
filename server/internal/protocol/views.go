package protocol

import (
	"github.com/your-org/blindmap/internal/game"
)

type SelfView struct {
	ID           game.PlayerID `json:"id"`
	Name         string        `json:"name"`
	Alive        bool          `json:"alive"`
	Inventory    []game.Item   `json:"inventory"`
	VisitedCount int           `json:"visitedCount"`
	TotalCells   int           `json:"totalCells"`
	InfoBlackout bool          `json:"infoBlackout"`
	SubmitsLeft  int           `json:"submitsLeft"`
}

// MapStats is aggregate-only map info (counts, no positions) — safe to share
// with every player. Backs the client "Info" button.
type MapStats struct {
	MapSize int            `json:"mapSize"`
	Counts  map[string]int `json:"counts"`
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
	MapStats    *MapStats         `json:"mapStats,omitempty"`
	Paused      bool              `json:"paused"`
}

// buildMapStats aggregates cell-kind counts. portal_a + portal_b collapse to
// "portal"; empty maps to "blank".
func buildMapStats(state *game.GameState) *MapStats {
	if len(state.Grid) == 0 {
		return nil
	}
	counts := map[string]int{
		"wall": 0, "blank": 0, "trap": 0, "reward": 0, "bullet": 0, "portal": 0, "info": 0,
	}
	for _, row := range state.Grid {
		for _, cell := range row {
			switch cell.Kind {
			case game.CellWall:
				counts["wall"]++
			case game.CellEmpty:
				counts["blank"]++
			case game.CellTrap:
				counts["trap"]++
			case game.CellReward:
				counts["reward"]++
			case game.CellBullet:
				counts["bullet"]++
			case game.CellPortalA, game.CellPortalB:
				counts["portal"]++
			case game.CellInfo:
				counts["info"]++
			}
		}
	}
	return &MapStats{MapSize: state.MapSize, Counts: counts}
}

// extractActorName pulls the first recognisable player name from a set of events.
func extractActorName(events []Event) string {
	keys := []string{"playerName", "byPlayerName"}
	for _, e := range events {
		switch m := e.Payload.(type) {
		case map[string]interface{}:
			for _, k := range keys {
				if v, ok := m[k].(string); ok && v != "" {
					return v
				}
			}
		case map[string]string:
			for _, k := range keys {
				if v := m[k]; v != "" {
					return v
				}
			}
		}
	}
	return "Someone"
}

func BuildPlayerView(state *game.GameState, playerID game.PlayerID, events []Event) PlayerView {
	p := state.Players[playerID]

	self := SelfView{
		ID:           p.ID,
		Name:         p.Name,
		Alive:        p.Alive,
		Inventory:    p.Inventory,
		VisitedCount: len(p.VisitedCells),
		TotalCells:   state.MapSize * state.MapSize,
		InfoBlackout: p.InfoBlackout,
		SubmitsLeft:  p.MaxSubmit,
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

	// Pass 1: when InfoBlackout is active suppress ALL events and replace with one
	// generic notice per actor so the player knows someone acted but learns nothing.
	filteredEvents := events
	if p.InfoBlackout && len(events) > 0 {
		actor := extractActorName(events)
		filteredEvents = []Event{{
			Kind:    "blackout_info",
			Payload: map[string]string{"playerName": actor},
		}}
		p.InfoBlackout = false
	} else if p.InfoBlackout {
		p.InfoBlackout = false
	}

	// Pass 2: apply per-player privacy rules.
	//   ForPlayerID != ""  → event is private; only the named player sees it.
	//   ActingPlayerID != "" → observers see the event but with private payload
	//                          fields (e.g. bulletFull) removed.
	resolved := make([]Event, 0, len(filteredEvents))
	for _, e := range filteredEvents {
		if e.ForPlayerID != "" && e.ForPlayerID != playerID {
			continue // private event for someone else
		}
		if e.ActingPlayerID != "" && e.ActingPlayerID != playerID {
			if m, ok := e.Payload.(map[string]interface{}); ok {
				stripped := make(map[string]interface{}, len(m))
				for k, v := range m {
					stripped[k] = v
				}
				delete(stripped, "bulletFull")
				e = Event{Kind: e.Kind, Payload: stripped}
			}
		}
		resolved = append(resolved, e)
	}
	filteredEvents = resolved

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
		MapStats:    buildMapStats(state),
		Paused:      state.Paused,
	}
}

