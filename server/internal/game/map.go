package game

import "math/rand/v2"

func GenerateMap(mapSize int, rng *rand.Rand) [][]Cell {
	grid := make([][]Cell, mapSize)
	for y := 0; y < mapSize; y++ {
		grid[y] = make([]Cell, mapSize)
		for x := 0; x < mapSize; x++ {
			grid[y][x] = Cell{Pos: Position{X: x, Y: y}, Kind: CellEmpty}
		}
	}

	total := mapSize * mapSize
	placeRandom(grid, mapSize, rng, CellBullet, total/10, 0)
	placeRandom(grid, mapSize, rng, CellReward, total/20, 0)
	placeRandom(grid, mapSize, rng, CellTrap, total/33, 0)

	portalPairs := 1 + rng.IntN(2) // 1 or 2 pairs
	for i := 1; i <= portalPairs; i++ {
		posA := randomEmptyCell(grid, mapSize, rng)
		posB := randomEmptyCell(grid, mapSize, rng)
		grid[posA.Y][posA.X] = Cell{Pos: posA, Kind: CellPortalA, PortalID: i}
		grid[posB.Y][posB.X] = Cell{Pos: posB, Kind: CellPortalB, PortalID: i}
	}

	return grid
}

func placeRandom(grid [][]Cell, mapSize int, rng *rand.Rand, kind CellKind, count int, _ int) {
	placed := 0
	attempts := 0
	for placed < count && attempts < count*10 {
		attempts++
		pos := randomEmptyCell(grid, mapSize, rng)
		if grid[pos.Y][pos.X].Kind == CellEmpty {
			grid[pos.Y][pos.X] = Cell{Pos: pos, Kind: kind}
			placed++
		}
	}
}

func randomEmptyCell(grid [][]Cell, mapSize int, rng *rand.Rand) Position {
	for {
		x := rng.IntN(mapSize)
		y := rng.IntN(mapSize)
		if grid[y][x].Kind == CellEmpty {
			return Position{X: x, Y: y}
		}
	}
}
