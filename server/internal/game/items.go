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

	// Move reward tile to a new random empty non-special position
	oldPos := p.Pos
	newPos := randomEmptyCell(state.Grid, state.MapSize, rng)
	state.Grid[oldPos.Y][oldPos.X] = Cell{Pos: oldPos, Kind: CellEmpty}
	state.Grid[newPos.Y][newPos.X] = Cell{Pos: newPos, Kind: CellReward}

	return events
}

func ResolveTrap(state *GameState, p *Player, rng *rand.Rand) []GameEvent {
	effect := rng.IntN(5)
	var events []GameEvent

	switch effect {
	case 0: // reveal own position to all
		events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
			"effect":     "reveal_position",
			"playerId":   string(p.ID),
			"playerName": p.Name,
			"pos":        p.Pos,
		}})
	case 1: // random teleport
		newPos := randomEmptyCell(state.Grid, state.MapSize, rng)
		p.Pos = newPos
		p.VisitedCells[newPos] = true
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
	case 4: // lose info next turn (blackout)
		p.InfoBlackout = true
		events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
			"effect": "info_blackout",
		}})
	}

	return events
}

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
	return nil
}

func ResolveCompass(state *GameState, p *Player, rng *rand.Rand) GameEvent {
	clue := rng.IntN(4)
	switch clue {
	case 0:
		dir := nearestPlayerDirection(state, p)
		return GameEvent{Kind: "clue_received", Payload: map[string]interface{}{
			"type": "nearest_direction", "direction": dir,
		}}
	case 1:
		return GameEvent{Kind: "clue_received", Payload: map[string]interface{}{
			"type": "own_start_pos", "pos": p.StartPos,
		}}
	case 2:
		// pick a random other alive player
		for _, other := range state.Players {
			if other.ID != p.ID && other.Alive {
				return GameEvent{Kind: "clue_received", Payload: map[string]interface{}{
					"type":       "other_player_pos",
					"playerName": other.Name,
					"pos":        other.Pos,
				}}
			}
		}
		// fallback to nearest direction if no other player
		dir := nearestPlayerDirection(state, p)
		return GameEvent{Kind: "clue_received", Payload: map[string]interface{}{
			"type": "nearest_direction", "direction": dir,
		}}
	default: // 3x3 surroundings
		cells := surroundingCells(state, p.Pos, 1)
		return GameEvent{Kind: "clue_received", Payload: map[string]interface{}{
			"type": "surroundings_3x3", "cells": cells,
		}}
	}
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
			return "E"
		}
		return "W"
	}
	if dy > 0 {
		return "S"
	}
	return "N"
}

func surroundingCells(state *GameState, center Position, radius int) []map[string]interface{} {
	var result []map[string]interface{}
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			x, y := center.X+dx, center.Y+dy
			if x < 0 || x >= state.MapSize || y < 0 || y >= state.MapSize {
				continue
			}
			cell := state.Grid[y][x]
			result = append(result, map[string]interface{}{
				"pos":  Position{X: x, Y: y},
				"kind": fmt.Sprintf("%s", cell.Kind),
			})
		}
	}
	return result
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
