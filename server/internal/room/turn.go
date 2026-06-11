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

// AdvanceTurn resolves a turn-consuming action (move or shoot). submit_map is
// handled separately by the room since it does not consume a turn.
func AdvanceTurn(state *game.GameState, rng *rand.Rand, playerID game.PlayerID, data *protocol.ActionData) ([]protocol.Event, error) {
	p, ok := state.Players[playerID]
	if !ok {
		return nil, newActionError("PLAYER_NOT_FOUND", "player not found")
	}

	switch data.Kind {
	case protocol.ActionMove:
		return applyMove(state, rng, p, game.Direction(data.Direction))
	case protocol.ActionShoot:
		return applyShoot(state, p, game.Direction(data.Direction))
	case protocol.ActionNukeTarget:
		return applyNukeTarget(state, p, data.NukeX, data.NukeY)
	case protocol.ActionPass:
		return []protocol.Event{}, nil
	default:
		return nil, newActionError("UNKNOWN_ACTION", fmt.Sprintf("unknown action: %s", data.Kind))
	}
}

func applyMove(state *game.GameState, rng *rand.Rand, p *game.Player, dir game.Direction) ([]protocol.Event, error) {
	dx, dy, ok := dirDelta(dir)
	if !ok {
		return nil, newActionError("INVALID_DIRECTION", fmt.Sprintf("invalid direction: %s", dir))
	}

	friendly := friendlyDirection(dir)
	newPos := game.Position{X: p.Pos.X + dx, Y: p.Pos.Y + dy}

	// Out of bounds or interior wall → wasted turn. Broadcast a failed move;
	// the event never distinguishes border from wall (others must deduce).
	if !inBounds(newPos, state.MapSize) || state.Grid[newPos.Y][newPos.X].Kind == game.CellWall {
		return []protocol.Event{{
			Kind: protocol.EventPlayerMoved,
			Payload: map[string]interface{}{
				"playerName": p.Name,
				"direction":  friendly,
				"success":    false,
			},
		}}, nil
	}

	p.Pos = newPos
	p.VisitedCells[newPos] = true

	cell := state.Grid[newPos.Y][newPos.X]
	blockType := "blank"
	bulletFull := false
	var gameEvents []game.GameEvent

	switch cell.Kind {
	case game.CellBullet:
		blockType = "bullet"
		if hasBullet(p) {
			bulletFull = true // already holding one — capped at 1
		} else {
			p.Inventory = append(p.Inventory, game.Item{
				ID:   fmt.Sprintf("bullet-%d-%d", newPos.X, newPos.Y),
				Kind: game.ItemBullet,
			})
		}
	case game.CellReward:
		blockType = "reward"
		gameEvents = game.ResolveReward(state, p, rng)
	case game.CellTrap:
		blockType = "trap"
		gameEvents = game.ResolveTrap(state, p, rng)
	case game.CellInfo:
		blockType = "info"
		ev := game.ResolveInfo(state, p, rng)
		gameEvents = append(gameEvents, ev)
	case game.CellPortalA, game.CellPortalB:
		blockType = "portal"
		gameEvents = game.ResolvePortal(state, p, rng)
	}

	moveEvent := protocol.Event{
		Kind: protocol.EventPlayerMoved,
		Payload: map[string]interface{}{
			"playerName": p.Name,
			"direction":  friendly,
			"success":    true,
			"blockType":  blockType,
			"bulletFull": bulletFull,
		},
		ActingPlayerID: p.ID, // bulletFull is stripped for observers in BuildPlayerView
	}

	return append([]protocol.Event{moveEvent}, toProtocolEvents(gameEvents)...), nil
}

func applyShoot(state *game.GameState, shooter *game.Player, dir game.Direction) ([]protocol.Event, error) {
	dx, dy, ok := dirDelta(dir)
	if !ok {
		return nil, newActionError("INVALID_DIRECTION", fmt.Sprintf("invalid direction: %s", dir))
	}

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
		Kind: protocol.EventShotFired,
		Payload: map[string]interface{}{
			"byPlayerId":   string(shooter.ID),
			"byPlayerName": shooter.Name,
			"direction":    friendlyDirection(dir),
		},
	}}

	pos := shooter.Pos
	for {
		next := game.Position{X: pos.X + dx, Y: pos.Y + dy}
		// Bullet stops at the map border and at interior walls.
		if !inBounds(next, state.MapSize) || state.Grid[next.Y][next.X].Kind == game.CellWall {
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

func applyNukeTarget(state *game.GameState, p *game.Player, topX, topY int) ([]protocol.Event, error) {
	if !p.PendingNuke {
		return nil, newActionError("NO_PENDING_NUKE", "no nuke pending")
	}
	gevs := game.ApplyNuke(state, p, topX, topY)
	state.Paused = false

	// Check last-alive win condition.
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

	return toProtocolEvents(gevs), nil
}

// applySubmitMap compares a player's reconstructed wall set against the real
// one. Exact match wins; otherwise it costs a submit attempt. Never costs a
// turn — the room calls this outside the turn flow.
func applySubmitMap(state *game.GameState, p *game.Player, walls []game.Position) ([]protocol.Event, error) {
	if p.MaxSubmit <= 0 {
		return nil, newActionError("NO_SUBMIT_LEFT", "no submit attempts remaining")
	}

	realWalls := make(map[game.Position]bool)
	for y, row := range state.Grid {
		for x, cell := range row {
			if cell.Kind == game.CellWall {
				realWalls[game.Position{X: x, Y: y}] = true
			}
		}
	}
	submitted := make(map[game.Position]bool)
	for _, w := range walls {
		if inBounds(w, state.MapSize) {
			submitted[w] = true
		}
	}

	// symmetric difference: walls missed + cells wrongly marked as wall
	wrong := 0
	for w := range realWalls {
		if !submitted[w] {
			wrong++
		}
	}
	for w := range submitted {
		if !realWalls[w] {
			wrong++
		}
	}

	if wrong == 0 {
		state.Phase = game.PhaseEnded
		state.Winner = &p.ID
		state.WinReason = "map_complete"
		return []protocol.Event{{
			Kind: protocol.EventMapSubmitted,
			Payload: map[string]interface{}{
				"playerName": p.Name,
				"correct":    true,
				"wrong":      0,
			},
		}}, nil
	}

	p.MaxSubmit--
	return []protocol.Event{{
		Kind: protocol.EventMapSubmitted,
		Payload: map[string]interface{}{
			"playerName":  p.Name,
			"correct":     false,
			"wrong":       wrong,
			"submitsLeft": p.MaxSubmit,
		},
	}}, nil
}

func hasBullet(p *game.Player) bool {
	for _, item := range p.Inventory {
		if item.Kind == game.ItemBullet {
			return true
		}
	}
	return false
}

func dirDelta(dir game.Direction) (int, int, bool) {
	switch dir {
	case game.DirN:
		return 0, -1, true
	case game.DirS:
		return 0, 1, true
	case game.DirE:
		return 1, 0, true
	case game.DirW:
		return -1, 0, true
	}
	return 0, 0, false
}

// friendlyDirection maps internal compass directions to the up/down/left/right
// vocabulary used in event payloads and the UI.
func friendlyDirection(dir game.Direction) string {
	switch dir {
	case game.DirN:
		return "up"
	case game.DirS:
		return "down"
	case game.DirE:
		return "right"
	case game.DirW:
		return "left"
	}
	return string(dir)
}

func inBounds(pos game.Position, mapSize int) bool {
	return pos.X >= 0 && pos.X < mapSize && pos.Y >= 0 && pos.Y < mapSize
}

func toProtocolEvents(gevs []game.GameEvent) []protocol.Event {
	result := make([]protocol.Event, 0, len(gevs))
	for _, e := range gevs {
		result = append(result, protocol.Event{
			Kind:           protocol.EventKind(e.Kind),
			Payload:        e.Payload,
			ForPlayerID:    e.ForPlayerID,
			ActingPlayerID: e.ActingPlayerID,
		})
	}
	return result
}
