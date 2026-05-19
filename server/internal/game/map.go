package game

import "math/rand/v2"

// GenerateMap builds a map with ~25% interior walls. It regenerates until the
// non-wall cells form a single connected region (no player can be trapped).
func GenerateMap(mapSize int, rng *rand.Rand) [][]Cell {
	for {
		grid := buildGrid(mapSize, rng)
		if isFullyConnected(grid, mapSize) {
			return grid
		}
	}
}

func buildGrid(mapSize int, rng *rand.Rand) [][]Cell {
	grid := make([][]Cell, mapSize)
	for y := 0; y < mapSize; y++ {
		grid[y] = make([]Cell, mapSize)
		for x := 0; x < mapSize; x++ {
			grid[y][x] = Cell{Pos: Position{X: x, Y: y}, Kind: CellEmpty}
		}
	}

	total := mapSize * mapSize
	// Walls first — placeRandom only overwrites CellEmpty, so every other
	// special cell placed afterwards naturally avoids walls.
	placeRandom(grid, mapSize, rng, CellWall, total/4, 0)
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

// isFullyConnected flood-fills from the first non-wall cell and verifies every
// non-wall cell is reachable.
func isFullyConnected(grid [][]Cell, mapSize int) bool {
	var start Position
	found := false
	nonWall := 0
	for y := 0; y < mapSize; y++ {
		for x := 0; x < mapSize; x++ {
			if grid[y][x].Kind != CellWall {
				nonWall++
				if !found {
					start = Position{X: x, Y: y}
					found = true
				}
			}
		}
	}
	if !found {
		return false
	}

	visited := make(map[Position]bool, nonWall)
	queue := []Position{start}
	visited[start] = true
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range [4]Position{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}} {
			n := Position{X: cur.X + d.X, Y: cur.Y + d.Y}
			if n.X < 0 || n.X >= mapSize || n.Y < 0 || n.Y >= mapSize {
				continue
			}
			if visited[n] || grid[n.Y][n.X].Kind == CellWall {
				continue
			}
			visited[n] = true
			queue = append(queue, n)
		}
	}
	return len(visited) == nonWall
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
