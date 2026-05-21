package game

import (
	"math"
	"math/rand/v2"
)

// DebugMapSize is the fixed size of the debug test map.
const DebugMapSize = 12

var debugPositions = map[string]Position{
	"Alice": {X: 1, Y: 1},
	"Bot1":  {X: 1, Y: 10},
	"Bot2":  {X: 10, Y: 1},
}

// DebugPositionByName returns the fixed start position for a named player in
// debug mode. Falls back to (1,1) for unknown names.
func DebugPositionByName(name string) Position {
	if pos, ok := debugPositions[name]; ok {
		return pos
	}
	return Position{X: 1, Y: 1}
}

// BuildDebugMap returns a fixed 12×12 map with every cell type present at
// known coordinates so tests can navigate deterministically.
//
// Layout (W=wall, .=empty, B=bullet, R=reward, T=trap, A=portalA, Z=portalB):
//
//	  0  1  2  3  4  5  6  7  8  9 10 11
//	0 W  W  W  W  W  W  W  W  W  W  W  W
//	1 W  .  .  B  R  T  .  .  .  .  .  W   ← Alice@(1,1), bullet@(3,1), reward@(4,1), trap@(5,1), Bot2@(10,1)
//	2 W  .  T  .  T  R  T  .  .  .  .  W   ← trap@(2,2), trap@(4,2), reward@(5,2), trap@(6,2)
//	3 W  .  .  R  .  T  .  .  .  .  .  W   ← reward@(3,3), trap@(5,3)
//	4 W  .  A  .  Z  .  .  .  .  .  .  W   ← portalA@(2,4), portalB@(4,4)
//	5 W  .  .  .  .  .  .  .  .  .  .  W
//	6 W  .  .  .  .  .  .  .  .  .  .  W
//	7 W  .  .  .  .  .  .  .  .  .  .  W
//	8 W  .  .  .  .  .  .  .  .  .  .  W
//	9 W  .  .  .  .  .  .  .  .  .  .  W
//	10 W .  .  .  .  .  .  .  .  .  .  W   ← Bot1@(1,10)
//	11 W  W  W  W  W  W  W  W  W  W  W  W
func BuildDebugMap() [][]Cell {
	raw := []string{
		"WWWWWWWWWWWW",
		"W..BRTT....W", // (3,1)=B (4,1)=R (5,1)=T (6,1)=T — Bot2@(10,1) placed by room
		"W.T.TRT....W", // (2,2)=T (4,2)=T (5,2)=R (6,2)=T
		"W..R.T.....W", // (3,3)=R (5,3)=T
		"W.AZ.......W", // (2,4)=portalA (3,4)=portalB
		"W..........W",
		"W..........W",
		"W..........W",
		"W..........W",
		"W..........W",
		"W..........W", // Bot1@(1,10) placed by room
		"WWWWWWWWWWWW",
	}
	// Use non-colliding portal IDs
	portalID := 1
	grid := make([][]Cell, DebugMapSize)
	for y := 0; y < DebugMapSize; y++ {
		grid[y] = make([]Cell, DebugMapSize)
		for x := 0; x < DebugMapSize; x++ {
			pos := Position{X: x, Y: y}
			var ch byte
			if y < len(raw) && x < len(raw[y]) {
				ch = raw[y][x]
			}
			switch ch {
			case 'W':
				grid[y][x] = Cell{Pos: pos, Kind: CellWall}
			case 'B':
				grid[y][x] = Cell{Pos: pos, Kind: CellBullet}
			case 'R':
				grid[y][x] = Cell{Pos: pos, Kind: CellReward}
			case 'T':
				grid[y][x] = Cell{Pos: pos, Kind: CellTrap}
			case 'A':
				grid[y][x] = Cell{Pos: pos, Kind: CellPortalA, PortalID: portalID}
			case 'Z':
				grid[y][x] = Cell{Pos: pos, Kind: CellPortalB, PortalID: portalID}
			default:
				grid[y][x] = Cell{Pos: pos, Kind: CellEmpty}
			}
		}
	}
	return grid
}

// GenerateMap builds a map with item counts scaled to playerCount.
// Regenerates until all non-wall cells form a single connected region.
func GenerateMap(mapSize int, rng *rand.Rand, wallPct float64, playerCount int) [][]Cell {
	if playerCount < 1 {
		playerCount = 1
	}
	for {
		grid := buildGrid(mapSize, rng, wallPct, playerCount)
		if isFullyConnected(grid, mapSize) {
			return grid
		}
	}
}

func buildGrid(mapSize int, rng *rand.Rand, wallPct float64, playerCount int) [][]Cell {
	grid := make([][]Cell, mapSize)
	for y := 0; y < mapSize; y++ {
		grid[y] = make([]Cell, mapSize)
		for x := 0; x < mapSize; x++ {
			grid[y][x] = Cell{Pos: Position{X: x, Y: y}, Kind: CellEmpty}
		}
	}

	total := mapSize * mapSize
	wallCount := int(float64(total) * wallPct)
	// Walls first; placeRandom only overwrites CellEmpty so all specials placed
	// afterward naturally avoid walls.
	placeRandom(grid, mapSize, rng, CellWall, wallCount, 0)

	// Bullets: 6–13% of total cells, at least one per player.
	bullets := clampMin(randBetween(rng, total*6/100, total*13/100), playerCount)
	placeRandom(grid, mapSize, rng, CellBullet, bullets, 0)

	// Rewards: 1.0–3.0× players; priority range 1.5–2.0×.
	rewards := playerScaled(rng, playerCount, 1.0, 3.0, 1.5, 2.0)
	placeRandom(grid, mapSize, rng, CellReward, rewards, 0)

	// Traps: 1.0–2.0× players; priority range 1.0–1.3× (light pressure).
	traps := playerScaled(rng, playerCount, 1.0, 2.0, 1.0, 1.3)
	placeRandom(grid, mapSize, rng, CellTrap, traps, 0)

	// Portals: 30% none, 35% 1 pair, 25% 2 pairs, 10% 3 pairs.
	pairs := randomPortalPairs(rng)
	for i := 1; i <= pairs; i++ {
		posA := randomEmptyCell(grid, mapSize, rng)
		posB := randomEmptyCell(grid, mapSize, rng)
		grid[posA.Y][posA.X] = Cell{Pos: posA, Kind: CellPortalA, PortalID: i}
		grid[posB.Y][posB.X] = Cell{Pos: posB, Kind: CellPortalB, PortalID: i}
	}

	return grid
}

// playerScaled returns a count that is a random multiple of playerCount.
// 80% of the time the multiplier is in [priorityMin, priorityMax];
// 20% spans the full [minFactor, maxFactor] range.
func playerScaled(rng *rand.Rand, playerCount int, minFactor, maxFactor, priorityMin, priorityMax float64) int {
	var factor float64
	if rng.Float64() < 0.80 {
		factor = priorityMin + rng.Float64()*(priorityMax-priorityMin)
	} else {
		factor = minFactor + rng.Float64()*(maxFactor-minFactor)
	}
	return clampMin(int(math.Round(factor*float64(playerCount))), 1)
}

func randBetween(rng *rand.Rand, lo, hi int) int {
	if lo >= hi {
		return lo
	}
	return lo + rng.IntN(hi-lo+1)
}

func clampMin(n, min int) int {
	if n < min {
		return min
	}
	return n
}

func randomPortalPairs(rng *rand.Rand) int {
	r := rng.Float64()
	switch {
	case r < 0.30:
		return 0
	case r < 0.65:
		return 1
	case r < 0.90:
		return 2
	default:
		return 3
	}
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
