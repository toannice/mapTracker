package game

import (
	"fmt"
	"math/rand/v2"
)

func ResolveReward(state *GameState, p *Player, rng *rand.Rand) []GameEvent {
	// weights: effect0=20%, effect1(nuke)=40%, effect2=40%
	effect := weightedRoll(rng, []int{20, 40, 40})
	var events []GameEvent

	switch effect {
	case 0: // reveal all current positions to everyone
		positions := make([]map[string]interface{}, 0, len(state.Players))
		for _, pl := range state.Players {
			if pl.Alive {
				positions = append(positions, map[string]interface{}{
					"name": pl.Name,
					"pos":  pl.Pos,
				})
			}
		}
		events = append(events, GameEvent{Kind: "reward_activated", Payload: map[string]interface{}{
			"effect": "all_positions_revealed", "positions": positions, "playerName": p.Name,
		}})
	case 1: // nuke — pause turn and wait for player to select target
		nukeSide := nukeSize(state.MapSize)
		p.PendingNuke = true
		p.NukeSide = nukeSide
		state.Paused = true
		events = append(events, GameEvent{Kind: "reward_activated", Payload: map[string]interface{}{
			"effect": "nuke_pending", "nukeSide": nukeSide, "playerName": p.Name,
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
			"effect": "all_bullet_locations", "locations": locs, "playerName": p.Name,
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
	return resolveTrap(state, p, rng, 0)
}

// trap weights: 0=reveal_position 15%, 1=random_teleport 20%, 2=lose_next_turn 40%,
//              3=lose_bullet 15%, 4=info_blackout 10%

func resolveTrap(state *GameState, p *Player, rng *rand.Rand, depth int) []GameEvent {
	effect := weightedRoll(rng, []int{15, 20, 40, 15, 10})
	var events []GameEvent

	switch effect {
	case 0: // reveal own position to all
		events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
			"effect": "reveal_position", "playerId": string(p.ID), "playerName": p.Name, "pos": p.Pos,
		}})
	case 1: // random teleport — then trigger destination cell (but not another teleport trap)
		newPos := randomEmptyCell(state.Grid, state.MapSize, rng)
		p.Pos = newPos
		p.VisitedCells[newPos] = true
		events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
			"effect": "random_teleport", "playerName": p.Name,
		}})
		events = append(events, resolveCellEffects(state, p, rng, state.Grid[newPos.Y][newPos.X], depth+1)...)
	case 2: // lose next turn
		p.SkipNextTurn = true
		events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
			"effect": "lose_next_turn", "playerName": p.Name,
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
			"effect": "lose_bullet", "lost": removed, "playerName": p.Name,
		}})
	case 4: // lose info next turn (blackout)
		p.InfoBlackout = true
		events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
			"effect": "info_blackout", "playerName": p.Name,
		}})
	}

	return events
}

func ResolvePortal(state *GameState, p *Player, rng *rand.Rand) []GameEvent {
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
				events := []GameEvent{{Kind: "portal_used", Payload: map[string]interface{}{
					"dest": dest,
				}}}
				events = append(events, resolveCellEffects(state, p, rng, state.Grid[dest.Y][dest.X], 1)...)
				return events
			}
		}
	}
	return nil
}

// resolveCellEffects triggers the effect of whatever cell the player just landed on
// (after a teleport or portal). depth prevents infinite trap→teleport→trap chains.
func resolveCellEffects(state *GameState, p *Player, rng *rand.Rand, cell Cell, depth int) []GameEvent {
	if depth > 3 {
		return nil
	}
	switch cell.Kind {
	case CellTrap:
		if depth > 0 {
			// Suppress random_teleport re-trigger to prevent infinite loops;
			// still allow all other trap effects.
			return resolveTrapNoTeleport(state, p, rng)
		}
		return resolveTrap(state, p, rng, depth)
	case CellReward:
		return ResolveReward(state, p, rng)
	case CellInfo:
		ev := ResolveInfo(state, p, rng)
		return []GameEvent{ev}
	case CellPortalA, CellPortalB:
		if depth > 0 {
			return nil // already inside a portal chain; one teleport per turn
		}
		return ResolvePortal(state, p, rng)
	}
	return nil
}

// resolveTrapNoTeleport resolves a trap but re-rolls if it would be random_teleport.
func resolveTrapNoTeleport(state *GameState, p *Player, rng *rand.Rand) []GameEvent {
	for {
		effect := rng.IntN(5)
		if effect == 1 { // skip random_teleport
			continue
		}
		// Borrow resolveTrap with a dummy effect by building the event inline.
		var events []GameEvent
		switch effect {
		case 0:
			events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
				"effect": "reveal_position", "playerId": string(p.ID), "playerName": p.Name, "pos": p.Pos,
			}})
		case 2:
			p.SkipNextTurn = true
			events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
				"effect": "lose_next_turn", "playerName": p.Name,
			}})
		case 3:
			removed := false
			for i, item := range p.Inventory {
				if item.Kind == ItemBullet {
					p.Inventory = append(p.Inventory[:i], p.Inventory[i+1:]...)
					removed = true
					break
				}
			}
			events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
				"effect": "lose_bullet", "lost": removed, "playerName": p.Name,
			}})
		case 4:
			p.InfoBlackout = true
			events = append(events, GameEvent{Kind: "trap_triggered", Payload: map[string]interface{}{
				"effect": "info_blackout", "playerName": p.Name,
			}})
		}
		return events
	}
}

// nukeSize returns the side length of the nuke square: floor(mapSize * 0.25), min 1.
func nukeSize(mapSize int) int {
	s := mapSize / 4
	if s < 1 {
		return 1
	}
	return s
}

// ApplyNuke kills all players inside the nukeSide×nukeSide area whose top-left
// corner is (topX, topY). Returns elimination events plus the nuke broadcast.
func ApplyNuke(state *GameState, shooter *Player, topX, topY int) []GameEvent {
	side := shooter.NukeSide
	shooter.PendingNuke = false
	shooter.NukeSide = 0

	// Clamp so the square stays inside the map.
	if topX+side > state.MapSize {
		topX = state.MapSize - side
	}
	if topY+side > state.MapSize {
		topY = state.MapSize - side
	}
	if topX < 0 {
		topX = 0
	}
	if topY < 0 {
		topY = 0
	}

	var eliminated []string
	var events []GameEvent
	for _, target := range state.Players {
		if !target.Alive {
			continue
		}
		if target.Pos.X >= topX && target.Pos.X < topX+side &&
			target.Pos.Y >= topY && target.Pos.Y < topY+side {
			target.Alive = false
			eliminated = append(eliminated, target.Name)
			events = append(events, GameEvent{Kind: "player_eliminated", Payload: map[string]string{
				"playerName": target.Name, "byPlayerName": shooter.Name,
			}})
		}
	}

	nukeEvent := GameEvent{Kind: "nuke_fired", Payload: map[string]interface{}{
		"byPlayerName": shooter.Name,
		"topX":         topX,
		"topY":         topY,
		"side":         side,
		"eliminated":   eliminated,
	}}
	return append([]GameEvent{nukeEvent}, events...)
}

func ResolveInfo(state *GameState, p *Player, rng *rand.Rand) GameEvent {
	// weights: clue0(nearest_dir)=40%, clue1=20%, clue2=20%, clue3=20%
	clue := weightedRoll(rng, []int{40, 20, 20, 20})
	var ev GameEvent
	switch clue {
	case 0:
		dir := nearestPlayerDirection(state, p)
		ev = GameEvent{Kind: "clue_received", Payload: map[string]interface{}{
			"type": "nearest_direction", "direction": dir,
		}}
	case 1:
		ev = GameEvent{Kind: "clue_received", Payload: map[string]interface{}{
			"type": "own_start_pos", "pos": p.StartPos,
		}}
	case 2:
		// collect all alive others, then pick one uniformly at random
		var candidates []*Player
		for _, other := range state.Players {
			if other.ID != p.ID && other.Alive {
				candidates = append(candidates, other)
			}
		}
		if len(candidates) > 0 {
			other := candidates[rng.IntN(len(candidates))]
			ev = GameEvent{Kind: "clue_received", Payload: map[string]interface{}{
				"type":       "other_player_pos",
				"playerName": other.Name,
				"pos":        other.Pos,
			}}
		}
		if ev.Kind == "" {
			// fallback to nearest direction if no other player
			dir := nearestPlayerDirection(state, p)
			ev = GameEvent{Kind: "clue_received", Payload: map[string]interface{}{
				"type": "nearest_direction", "direction": dir,
			}}
		}
	default: // 3x3 surroundings
		cells := surroundingCells(state, p.Pos, 1)
		ev = GameEvent{Kind: "clue_received", Payload: map[string]interface{}{
			"type": "surroundings_3x3", "cells": cells,
		}}
	}
	ev.ForPlayerID = p.ID // clue is private to the player who stepped on the compass
	return ev
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

func surroundingCells(state *GameState, center Position, radius int) []map[string]interface{} {
	var result []map[string]interface{}
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
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

// weightedRoll picks an index from weights (must sum to 100).
func weightedRoll(rng *rand.Rand, weights []int) int {
	total := 0
	for _, w := range weights {
		total += w
	}
	r := rng.IntN(total)
	cumulative := 0
	for i, w := range weights {
		cumulative += w
		if r < cumulative {
			return i
		}
	}
	return len(weights) - 1
}
