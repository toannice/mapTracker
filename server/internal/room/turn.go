package room

import (
	"fmt"
	"math/rand/v2"

	"github.com/your-org/blindmap/internal/game"
	"github.com/your-org/blindmap/internal/protocol"
)

type actionError struct {
	code string
	msg  string
}

func (e *actionError) Error() string { return e.msg }
func (e *actionError) Code() string  { return e.code }

func newActionError(code, msg string) error {
	return &actionError{code: code, msg: msg}
}

func AdvanceTurn(state *game.GameState, rng *rand.Rand, playerID game.PlayerID, data *protocol.ActionData) ([]protocol.Event, error) {
	p, ok := state.Players[playerID]
	if !ok {
		return nil, newActionError("PLAYER_NOT_FOUND", "player not found")
	}

	switch data.Kind {
	case protocol.ActionMove:
		return applyMove(state, rng, p, game.Direction(data.Direction))
	case protocol.ActionPickup:
		return applyPickup(state, p)
	case protocol.ActionShoot:
		return applyShoot(state, p, game.Direction(data.Direction))
	case protocol.ActionSubmitMap:
		return applySubmitMap(state, p)
	default:
		return nil, newActionError("UNKNOWN_ACTION", fmt.Sprintf("unknown action: %s", data.Kind))
	}
}

func applyMove(state *game.GameState, rng *rand.Rand, p *game.Player, dir game.Direction) ([]protocol.Event, error) {
	newPos, err := stepPosition(p.Pos, dir, state.MapSize)
	if err != nil {
		return nil, err
	}

	p.Pos = newPos
	p.VisitedCells[newPos] = true

	cell := state.Grid[newPos.Y][newPos.X]
	var gameEvents []game.GameEvent

	switch cell.Kind {
	case game.CellReward:
		gameEvents = game.ResolveReward(state, p, rng)
	case game.CellTrap:
		gameEvents = game.ResolveTrap(state, p, rng)
	case game.CellPortalA, game.CellPortalB:
		gameEvents = game.ResolvePortal(state, p)
	}

	return toProtocolEvents(gameEvents), nil
}

func applyPickup(state *game.GameState, p *game.Player) ([]protocol.Event, error) {
	cell := state.Grid[p.Pos.Y][p.Pos.X]
	if cell.Kind != game.CellBullet {
		return nil, newActionError("NOTHING_TO_PICKUP", "no item to pick up here")
	}
	itemID := fmt.Sprintf("bullet-%d-%d", p.Pos.X, p.Pos.Y)
	p.Inventory = append(p.Inventory, game.Item{ID: itemID, Kind: game.ItemBullet})
	// tile stays (infinite pickups)
	return []protocol.Event{}, nil
}

func applyShoot(state *game.GameState, shooter *game.Player, dir game.Direction) ([]protocol.Event, error) {
	bulletIdx := -1
	for i, item := range shooter.Inventory {
		if item.Kind == game.ItemBullet {
			bulletIdx = i
			break
		}
	}
	if bulletIdx < 0 {
		return nil, newActionError("NO_BULLET", "no bullet in inventory")
	}

	shooter.Inventory = append(shooter.Inventory[:bulletIdx], shooter.Inventory[bulletIdx+1:]...)

	events := []protocol.Event{{
		Kind:    protocol.EventShotFired,
		Payload: map[string]interface{}{"byPlayerId": string(shooter.ID), "direction": string(dir)},
	}}

	pos := shooter.Pos
	for {
		next, err := stepPosition(pos, dir, state.MapSize)
		if err != nil {
			break
		}
		pos = next

		for _, target := range state.Players {
			if target.Alive && target.ID != shooter.ID && target.Pos == pos {
				target.Alive = false
				events = append(events, protocol.Event{
					Kind: protocol.EventPlayerEliminated,
					Payload: map[string]string{
						"playerName":   target.Name,
						"byPlayerName": shooter.Name,
					},
				})

				aliveCount := 0
				var survivor game.PlayerID
				for id, pl := range state.Players {
					if pl.Alive {
						aliveCount++
						survivor = id
					}
				}
				if aliveCount == 1 {
					state.Phase = game.PhaseEnded
					state.Winner = &survivor
					state.WinReason = "last_alive"
				}
				return events, nil
			}
		}
	}

	return events, nil
}

func applySubmitMap(state *game.GameState, p *game.Player) ([]protocol.Event, error) {
	total := state.MapSize * state.MapSize
	visited := len(p.VisitedCells)
	if visited < total {
		return nil, newActionError("MAP_INCOMPLETE",
			fmt.Sprintf("%d cells remain", total-visited))
	}
	state.Phase = game.PhaseEnded
	state.Winner = &p.ID
	state.WinReason = "map_complete"
	return []protocol.Event{{Kind: protocol.EventMapSubmitted, Payload: map[string]string{"playerName": p.Name}}}, nil
}

func stepPosition(pos game.Position, dir game.Direction, mapSize int) (game.Position, error) {
	newPos := pos
	switch dir {
	case game.DirN:
		newPos.Y--
	case game.DirS:
		newPos.Y++
	case game.DirE:
		newPos.X++
	case game.DirW:
		newPos.X--
	default:
		return pos, newActionError("INVALID_DIRECTION", fmt.Sprintf("invalid direction: %s", dir))
	}
	if newPos.X < 0 || newPos.X >= mapSize || newPos.Y < 0 || newPos.Y >= mapSize {
		return pos, newActionError("INVALID_DIRECTION", "move out of bounds")
	}
	return newPos, nil
}

func toProtocolEvents(gevs []game.GameEvent) []protocol.Event {
	result := make([]protocol.Event, 0, len(gevs))
	for _, e := range gevs {
		result = append(result, protocol.Event{
			Kind:    protocol.EventKind(e.Kind),
			Payload: e.Payload,
		})
	}
	return result
}
