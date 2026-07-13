package game

import (
	"fmt"
	"math/rand/v2"
)

func ResolveReward(state *GameState, p *Player, rng *rand.Rand) []GameEvent {
	effect := rng.IntN(3)
	var events []GameEvent

	switch effect {
	case 0: // reveal all current positions to everyone
		type namedPos struct {
			Name string   `json:"name"`
			Pos  Position `json:"pos"`
		}
		var positions []namedPos
		for _, pl := range state.Players {
			if pl.Alive {
				positions = append(positions, namedPos{Name: pl.Name, Pos: pl.Pos})
			}
		}
		events = append(events, GameEvent{Kind: "reward_activated", Payload: map[string]interface{}{
			"effect": "all_positions_revealed", "positions": positions,
		}})
	case 1: // nearest player direction
		dir := nearestPlayerDirection(state, p)
		events = append(events, GameEvent{Kind: "reward_activated", Payload: map[string]interface{}{
			"effect": "nearest_direction", "direction": dir,
		}})
	case 2: // all bullet tile locations
		var locs []Position
		for y, row := range state.Grid {
			for x, cell := range row {
				if cell.Kind == CellBullet {
					locs = append(locs, Position{X: x, Y: y})
				}
			}
		}
		events = append(events, GameEvent{Kind: "reward_activated", Payload: map[string]interface{}{
			"effect": "all_bullet_locations", "locations": locs,
		}})
	}

	// Move reward tile to a new random empty non-special position.
	// safeRandomEmptyCell caps attempts so the game never hangs on a packed map.
	oldPos := p.Pos
	if newPos, ok := safeRandomEmptyCell(state.Grid, state.MapSize, rng); ok {
		state.Grid[oldPos.Y][oldPos.X] = Cell{Pos: oldPos, Kind: CellEmpty}
		state.Grid[newPos.Y][newPos.X] = Cell{Pos: newPos, Kind: CellReward}
	}
	// If no empty cell exists, the reward tile simply disappears — acceptable on
	// very dense maps.

	return events
}

func ResolveTrap(state *GameState, p *Player, rng *rand.Rand) []GameEvent {
	effect := rng.IntN(4)
	var events []GameEvent

	switch effect {
	case 0: // reveal own position to all
		events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
			"effect":     "reveal_position",
			"playerId":   string(p.ID),
			"playerName": p.Name,
			"pos":        p.Pos,
		}})
	case 1: // random teleport — capped to avoid infinite loop on packed maps
		if newPos, ok := safeRandomEmptyCell(state.Grid, state.MapSize, rng); ok {
			p.Pos = newPos
			p.VisitedCells[newPos] = true
		}
		events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
			"effect": "random_teleport",
		}})
	case 2: // lose next turn
		p.SkipNextTurn = true
		events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
			"effect": "lose_next_turn",
		}})
	case 3: // lose bullet
		removed := false
		for i, item := range p.Inventory {
			if item.Kind == ItemBullet {
				p.Inventory = append(p.Inventory[:i], p.Inventory[i+1:]...)
				removed = true
				break
			}
		}
		events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
			"effect": "lose_bullet", "lost": removed,
		}})
	}

	return events
}

// ResolveInfo handles stepping on an info cell.
// One of three effects is chosen randomly:
//
//	0. Reveal 3×3 surrounding cell types (relative positions, no absolute coords)
//	1. Reveal a random alive opponent's current position
//	2. Reveal the stepper's own start position
//
// All events are private (ForPlayerID set), so only the stepper sees the detail.
func ResolveInfo(state *GameState, p *Player, rng *rand.Rand) []GameEvent {
	effect := rng.IntN(3)
	var events []GameEvent

	switch effect {
	case 0: // 3×3 surroundings — relative offsets, no absolute position leaked
		type relCell struct {
			Rel  string   `json:"rel"`
			Kind CellKind `json:"kind"`
		}
		var cells []relCell
		for dr := -1; dr <= 1; dr++ {
			for dc := -1; dc <= 1; dc++ {
				if dr == 0 && dc == 0 {
					continue
				}
				nx, ny := p.Pos.X+dc, p.Pos.Y+dr
				kind := CellWall
				if nx >= 0 && nx < state.MapSize && ny >= 0 && ny < state.MapSize {
					kind = state.Grid[ny][nx].Kind
				}
				relRow := dr + 1
				relCol := dc + 1
				cells = append(cells, relCell{
					Rel:  fmt.Sprintf("%d-%d", relRow, relCol),
					Kind: kind,
				})
			}
		}
		events = append(events, GameEvent{
			Kind:        "info_revealed",
			ForPlayerID: p.ID,
			Payload: map[string]interface{}{
				"type":  "surroundings_3x3",
				"cells": cells,
			},
		})

	case 1: // random alive opponent's position
		var targets []*Player
		for _, pl := range state.Players {
			if pl.ID != p.ID && pl.Alive {
				targets = append(targets, pl)
			}
		}
		if len(targets) == 0 {
			// fallback: own start pos
			events = append(events, GameEvent{
				Kind:        "info_revealed",
				ForPlayerID: p.ID,
				Payload: map[string]interface{}{
					"type": "own_start",
					"pos":  p.StartPos,
				},
			})
			break
		}
		target := targets[rng.IntN(len(targets))]
		events = append(events, GameEvent{
			Kind:        "info_revealed",
			ForPlayerID: p.ID,
			Payload: map[string]interface{}{
				"type":       "player_position",
				"playerName": target.Name,
				"pos":        target.Pos,
			},
		})

	case 2: // own start position
		events = append(events, GameEvent{
			Kind:        "info_revealed",
			ForPlayerID: p.ID,
			Payload: map[string]interface{}{
				"type": "own_start",
				"pos":  p.StartPos,
			},
		})
	}

	return events
}

// ResolvePortal teleports the player to the partner portal.
// Always returns a portal_used event — even if the partner is missing (orphaned
// portal), so the player receives feedback instead of silent failure.
func ResolvePortal(state *GameState, p *Player) []GameEvent {
	cell := state.Grid[p.Pos.Y][p.Pos.X]
	partnerKind := CellPortalB
	if cell.Kind == CellPortalB {
		partnerKind = CellPortalA
	}
	for y, row := range state.Grid {
		for x, c := range row {
			if c.Kind == partnerKind && c.PortalID == cell.PortalID {
				dest := Position{X: x, Y: y}
				p.Pos = dest
				p.VisitedCells[dest] = true
				return []GameEvent{{Kind: "portal_used", Payload: map[string]interface{}{
					"dest": dest,
				}}}
			}
		}
	}
	// Orphaned portal: no partner found — player stays, event still fires.
	return []GameEvent{{Kind: "portal_used", Payload: map[string]interface{}{
		"dest": p.Pos,
	}}}
}

// safeRandomEmptyCell finds a random empty cell, capped at mapSize² × 3 random
// attempts followed by a full linear scan. Returns false only if no empty cell
// exists at all — callers should treat that as a no-op.
func safeRandomEmptyCell(grid [][]Cell, mapSize int, rng *rand.Rand) (Position, bool) {
	cap := mapSize * mapSize * 3
	for i := 0; i < cap; i++ {
		x := rng.IntN(mapSize)
		y := rng.IntN(mapSize)
		if grid[y][x].Kind == CellEmpty {
			return Position{X: x, Y: y}, true
		}
	}
	// Linear scan fallback — guarantees we find any remaining empty cell.
	for y, row := range grid {
		for x, cell := range row {
			if cell.Kind == CellEmpty {
				return Position{X: x, Y: y}, true
			}
		}
	}
	return Position{}, false
}

func nearestPlayerDirection(state *GameState, p *Player) string {
	minDist := int(^uint(0) >> 1)
	var dir string
	for _, other := range state.Players {
		if other.ID == p.ID || !other.Alive {
			continue
		}
		dx := other.Pos.X - p.Pos.X
		dy := other.Pos.Y - p.Pos.Y
		dist := abs(dx) + abs(dy)
		if dist < minDist {
			minDist = dist
			dir = compassDir(dx, dy)
		}
	}
	if dir == "" {
		return "none"
	}
	return dir
}

func compassDir(dx, dy int) string {
	if abs(dx) >= abs(dy) {
		if dx > 0 {
			return "right"
		}
		return "left"
	}
	if dy > 0 {
		return "down"
	}
	return "up"
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
