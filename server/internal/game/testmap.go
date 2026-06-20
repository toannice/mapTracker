package game

import "time"

const TestRoomID  RoomID = "TEST01"
const TestRoomID2 RoomID = "TEST02"
const TestPlayerAliceID PlayerID = "test-alice-000001"
const TestPlayerBotID   PlayerID = "test-bot1-000001"

// BuildTestMap4x4 returns a fixed 4×4 grid for automated testing.
// All special cells have ForceEffect (and ForcePos for teleport) so outcomes
// are fully deterministic — no random roll, no seed dependency.
//
// Layout (X→, Y↓):
//
//	      X=0          X=1                  X=2                    X=3
//	Y=0  [Alice]       trap:reveal_pos      trap:lose_bullet       trap:lose_next_turn
//	Y=1  info:own_start reward:all_pos_rev  reward:all_bullets     reward:nuke_pending
//	Y=2  info:other_pos info:surroundings   trap:info_blackout     trap:teleport→(2,3)
//	Y=3  [Bot2 NPC]    empty                empty                  empty
//
// Golden-replay path (Alice solo, 15 moves):
//
//	→→→↓←←←↓→→→↓←↓ shoot←
//	Visits every special cell once; teleport at (3,2) lands at (2,3).
func BuildTestMap4x4() [][]Cell {
	grid := make([][]Cell, 4)
	for y := 0; y < 4; y++ {
		grid[y] = make([]Cell, 4)
		for x := 0; x < 4; x++ {
			grid[y][x] = Cell{Pos: Position{X: x, Y: y}, Kind: CellEmpty}
		}
	}

	set := func(x, y int, k CellKind, fe string, fp *Position) {
		grid[y][x] = Cell{Pos: Position{X: x, Y: y}, Kind: k, ForceEffect: fe, ForcePos: fp}
	}

	dest23 := &Position{X: 2, Y: 3}

	// Row 0 — traps (3 of 5 effects)
	set(1, 0, CellTrap, "reveal_position", nil)
	set(2, 0, CellTrap, "lose_bullet", nil)
	set(3, 0, CellTrap, "lose_next_turn", nil)

	// Row 1 — rewards (all 3 effects) + info
	set(0, 1, CellInfo,   "own_start_pos", nil)
	set(1, 1, CellReward, "all_positions_revealed", nil)
	set(2, 1, CellReward, "all_bullet_locations", nil)
	set(3, 1, CellReward, "nuke_pending", nil)

	// Row 2 — info (3 of 4 effects, skip nearest_direction) + traps (2 of 5)
	set(0, 2, CellInfo,   "other_player_pos", nil)
	set(1, 2, CellInfo,   "surroundings_3x3", nil)
	set(2, 2, CellTrap,   "info_blackout", nil)
	set(3, 2, CellTrap,   "random_teleport", dest23)

	// Row 3 — all empty (Bot2 NPC placed separately by test)
	return grid
}

// BuildTestBot2NPC returns the Bot2 NPC pre-placed at (0,3).
func BuildTestBot2NPC() *Player {
	pos := Position{X: 0, Y: 3}
	p := &Player{
		ID:           "dummy-bot2",
		Name:         "Bot2",
		Alive:        true,
		Inventory:    []Item{},
		MaxSubmit:    0,
		VisitedCells: make(map[Position]bool),
		ConnectedAt:  time.Now(),
		LastSeen:     time.Now(),
	}
	p.Pos = pos
	p.StartPos = pos
	p.VisitedCells[pos] = true
	return p
}
