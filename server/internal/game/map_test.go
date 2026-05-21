package game

import (
	"math/rand/v2"
	"testing"
)

func TestGenerateMapCellCounts(t *testing.T) {
	mapSize := 10
	rng := rand.New(rand.NewPCG(42, 0))
	grid := GenerateMap(mapSize, rng, 0.25)

	if len(grid) != mapSize {
		t.Errorf("rows: want %d, got %d", mapSize, len(grid))
	}
	for _, row := range grid {
		if len(row) != mapSize {
			t.Errorf("cols: want %d, got %d", mapSize, len(row))
		}
	}

	// Verify all cells have valid positions
	for y, row := range grid {
		for x, cell := range row {
			if cell.Pos.X != x || cell.Pos.Y != y {
				t.Errorf("cell pos mismatch at (%d,%d): got %+v", x, y, cell.Pos)
			}
		}
	}
}

func TestGenerateMapIsConnected(t *testing.T) {
	for seed := uint64(0); seed < 30; seed++ {
		rng := rand.New(rand.NewPCG(seed, seed))
		grid := GenerateMap(8, rng, 0.25)
		if !isFullyConnected(grid, 8) {
			t.Fatalf("seed %d: generated map is not fully connected", seed)
		}
	}
}

func TestVisitedCellsDedup(t *testing.T) {
	visited := make(map[Position]bool)
	pos := Position{X: 1, Y: 1}
	visited[pos] = true
	visited[pos] = true // duplicate

	if len(visited) != 1 {
		t.Errorf("want 1 unique cell, got %d", len(visited))
	}
}
