package game

import "testing"

// Wire-security regression guard: portal_used must never broadcast the
// destination — that would reveal the teleported player's position to everyone.
func TestResolvePortalHidesDestination(t *testing.T) {
	const size = 3
	grid := make([][]Cell, size)
	for y := 0; y < size; y++ {
		grid[y] = make([]Cell, size)
		for x := 0; x < size; x++ {
			grid[y][x] = Cell{Pos: Position{X: x, Y: y}, Kind: CellEmpty}
		}
	}
	grid[0][0] = Cell{Pos: Position{X: 0, Y: 0}, Kind: CellPortalA, PortalID: 1}
	grid[2][2] = Cell{Pos: Position{X: 2, Y: 2}, Kind: CellPortalB, PortalID: 1}

	p := &Player{
		ID: "p1", Name: "Alice", Alive: true,
		Pos:          Position{X: 0, Y: 0},
		VisitedCells: map[Position]bool{{X: 0, Y: 0}: true},
	}
	state := &GameState{MapSize: size, Grid: grid, Players: map[PlayerID]*Player{"p1": p}}

	events := ResolvePortal(state, p)

	if p.Pos != (Position{X: 2, Y: 2}) {
		t.Fatalf("player not teleported to partner portal, pos=%+v", p.Pos)
	}
	if len(events) != 1 || events[0].Kind != "portal_used" {
		t.Fatalf("expected one portal_used event, got %+v", events)
	}
	if events[0].ForPlayerID != "" {
		t.Error("portal_used should be public (feedback that a teleport happened)")
	}
	payload, ok := events[0].Payload.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected payload type %T", events[0].Payload)
	}
	if _, leaked := payload["dest"]; leaked {
		t.Error("portal_used payload must NOT contain dest — position leak")
	}
	if payload["playerName"] != "Alice" {
		t.Errorf("payload should name the player, got %v", payload["playerName"])
	}
}
