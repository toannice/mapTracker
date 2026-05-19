package protocol

import (
	"testing"

	"github.com/your-org/blindmap/internal/game"
)

func TestBuildPlayerViewNoPositionLeak(t *testing.T) {
	aliceID := game.PlayerID("alice")
	bobID := game.PlayerID("bob")

	state := &game.GameState{
		RoomID:      "TEST01",
		Phase:       game.PhaseActive,
		MapSize:     5,
		TurnSeconds: 30,
		Players: map[game.PlayerID]*game.Player{
			aliceID: {
				ID:           aliceID,
				Name:         "Alice",
				Pos:          game.Position{X: 1, Y: 2},
				Alive:        true,
				Inventory:    []game.Item{},
				VisitedCells: map[game.Position]bool{{X: 1, Y: 2}: true},
			},
			bobID: {
				ID:           bobID,
				Name:         "Bob",
				Pos:          game.Position{X: 3, Y: 4}, // Bob's secret position
				Alive:        true,
				Inventory:    []game.Item{{ID: "b1", Kind: game.ItemBullet}},
				VisitedCells: map[game.Position]bool{{X: 3, Y: 4}: true},
			},
		},
		Grid:     makeGrid(5),
		TurnOrder: []game.PlayerID{aliceID, bobID},
	}

	// Alice's view must never reveal Bob's position
	view := BuildPlayerView(state, aliceID, nil)

	// Verify Alice sees herself correctly
	if view.Self.Pos.X != 1 || view.Self.Pos.Y != 2 {
		t.Errorf("Alice self pos: want (1,2), got (%d,%d)", view.Self.Pos.X, view.Self.Pos.Y)
	}

	// Verify OtherPlayerView for Bob has NO position field
	var bobView *OtherPlayerView
	for _, other := range view.Others {
		if other.ID == bobID {
			bobView = &other
			break
		}
	}
	if bobView == nil {
		t.Fatal("Bob not found in others")
	}
	// OtherPlayerView only has ID, Name, Alive — no position
	if bobView.Name != "Bob" {
		t.Errorf("Bob name: want Bob, got %s", bobView.Name)
	}
	if !bobView.Alive {
		t.Error("Bob should be alive")
	}
	// OtherPlayerView struct has no Pos field — compile-time guarantee
}

func TestBuildPlayerViewVisitedOnly(t *testing.T) {
	aliceID := game.PlayerID("alice")
	state := &game.GameState{
		RoomID:      "TEST01",
		Phase:       game.PhaseActive,
		MapSize:     5,
		TurnSeconds: 30,
		Players: map[game.PlayerID]*game.Player{
			aliceID: {
				ID:    aliceID,
				Name:  "Alice",
				Pos:   game.Position{X: 0, Y: 0},
				Alive: true,
				VisitedCells: map[game.Position]bool{
					{X: 0, Y: 0}: true,
					{X: 1, Y: 0}: true,
				},
				Inventory: []game.Item{},
			},
		},
		Grid:      makeGrid(5),
		TurnOrder: []game.PlayerID{aliceID},
	}

	view := BuildPlayerView(state, aliceID, nil)

	if len(view.VisibleMap) != 2 {
		t.Errorf("visible cells: want 2, got %d", len(view.VisibleMap))
	}
	if view.Self.VisitedCount != 2 {
		t.Errorf("visitedCount: want 2, got %d", view.Self.VisitedCount)
	}
}

func makeGrid(size int) [][]game.Cell {
	grid := make([][]game.Cell, size)
	for y := 0; y < size; y++ {
		grid[y] = make([]game.Cell, size)
		for x := 0; x < size; x++ {
			grid[y][x] = game.Cell{Pos: game.Position{X: x, Y: y}, Kind: game.CellEmpty}
		}
	}
	return grid
}
