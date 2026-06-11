package game

import "time"

const TestRoomID  RoomID = "TEST01"
const TestRoomID2 RoomID = "TEST02"
const TestPlayerAliceID PlayerID = "test-alice-000001"
const TestPlayerBotID   PlayerID = "test-bot1-000001"

// BuildTestMap4x4 returns a fixed 4×4 grid for automated testing.
// Layout (X, Y):
//
//	X→  0         1        2        3
//	Y=0 [Alice]   trap     trap     trap
//	Y=1 info      reward   reward   reward
//	Y=2 info      info     empty    empty
//	Y=3 [Bot2NPC] [Bot1]   bullet   wall
func BuildTestMap4x4() [][]Cell {
	grid := make([][]Cell, 4)
	for y := 0; y < 4; y++ {
		grid[y] = make([]Cell, 4)
		for x := 0; x < 4; x++ {
			grid[y][x] = Cell{Pos: Position{X: x, Y: y}, Kind: CellEmpty}
		}
	}
	set := func(x, y int, k CellKind) {
		grid[y][x] = Cell{Pos: Position{X: x, Y: y}, Kind: k}
	}
	set(1, 0, CellTrap)
	set(2, 0, CellTrap)
	set(3, 0, CellTrap)
	set(0, 1, CellInfo)
	set(1, 1, CellReward)
	set(2, 1, CellReward)
	set(3, 1, CellReward)
	set(0, 2, CellInfo)
	set(1, 2, CellInfo)
	set(2, 3, CellBullet)
	set(3, 3, CellWall)
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
