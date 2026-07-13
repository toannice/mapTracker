package bot

import (
	"math/rand/v2"
	"testing"

	"github.com/your-org/blindmap/internal/game"
	"github.com/your-org/blindmap/internal/protocol"
)

func testRng() *rand.Rand { return rand.New(rand.NewPCG(42, 43)) }

func makeView(mapSize int, selfPos game.Position, visible []protocol.CellView, events []protocol.Event) *protocol.PlayerView {
	return &protocol.PlayerView{
		Self: protocol.SelfView{
			ID: "bot-1", Name: "Bot1 (hard)", Pos: selfPos, Alive: true,
			Inventory: []game.Item{}, SubmitsLeft: 3,
		},
		VisibleMap: visible,
		Events:     events,
		MapStats: &protocol.MapStats{
			MapSize: mapSize,
			Counts:  map[string]int{"wall": 2, "blank": mapSize*mapSize - 2},
		},
	}
}

func TestParseDifficulty(t *testing.T) {
	cases := []struct {
		in   string
		want Difficulty
		ok   bool
	}{
		{"easy", Easy, true},
		{"medium", Medium, true},
		{"hard", Hard, true},
		{"", Medium, true},
		{"impossible", "", false},
	}
	for _, c := range cases {
		got, ok := ParseDifficulty(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseDifficulty(%q) = (%v, %v), want (%v, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestDecideAlwaysReturnsTurnAction(t *testing.T) {
	for _, diff := range []Difficulty{Easy, Medium, Hard} {
		b := New("bot-1", "Bot1 (hard)", diff)
		view := makeView(5, game.Position{X: 2, Y: 2},
			[]protocol.CellView{{Pos: game.Position{X: 2, Y: 2}, Kind: game.CellEmpty}}, nil)
		actions := b.Decide(view, testRng())
		if len(actions) == 0 {
			t.Fatalf("%s: Decide returned no actions", diff)
		}
		last := actions[len(actions)-1]
		switch last.Kind {
		case protocol.ActionMove, protocol.ActionShoot, protocol.ActionPass:
		default:
			t.Errorf("%s: last action must consume the turn, got %s", diff, last.Kind)
		}
		if last.Kind == protocol.ActionMove {
			dx, dy := compassDelta(last.Direction)
			if dx == 0 && dy == 0 {
				t.Errorf("%s: move with invalid direction %q", diff, last.Direction)
			}
		}
	}
}

func TestWallInferenceFromFailedMove(t *testing.T) {
	b := New("bot-1", "Bot1 (hard)", Hard)
	view := makeView(5, game.Position{X: 2, Y: 2},
		[]protocol.CellView{{Pos: game.Position{X: 2, Y: 2}, Kind: game.CellEmpty}}, nil)
	actions := b.Decide(view, testRng())
	last := actions[len(actions)-1]
	if last.Kind != protocol.ActionMove {
		t.Fatalf("expected a move, got %s", last.Kind)
	}
	target := *b.pendingMoveTarget

	// Server reports the move failed → target must be recorded as a wall.
	fail := makeView(5, game.Position{X: 2, Y: 2}, nil, []protocol.Event{{
		Kind: protocol.EventPlayerMoved,
		Payload: map[string]interface{}{
			"playerName": "Bot1 (hard)", "direction": "up", "success": false,
		},
	}})
	b.Observe(fail)
	if !b.knownWalls[target] {
		t.Errorf("failed move target %+v not recorded as wall", target)
	}
	if b.pendingMoveTarget != nil {
		t.Error("pendingMoveTarget should be cleared after observation")
	}
}

func TestMediumAvoidsKnownWalls(t *testing.T) {
	b := New("bot-1", "Bot1 (med)", Medium)
	b.Name = "Bot1 (med)"
	// 3×3 map, bot at center, walls N and S → exploration must go E or W.
	view := makeView(3, game.Position{X: 1, Y: 1},
		[]protocol.CellView{{Pos: game.Position{X: 1, Y: 1}, Kind: game.CellEmpty}}, nil)
	b.Observe(view)
	b.knownWalls[game.Position{X: 1, Y: 0}] = true
	b.knownWalls[game.Position{X: 1, Y: 2}] = true

	rng := testRng()
	for i := 0; i < 20; i++ {
		dir := b.exploreDirection(rng, true, false)
		if dir == "N" || dir == "S" {
			t.Fatalf("exploration stepped into a known wall: %s", dir)
		}
	}
}

func TestHardSubmitsWhenWallSetComplete(t *testing.T) {
	b := New("bot-1", "Bot1 (hard)", Hard)
	view := makeView(5, game.Position{X: 2, Y: 2},
		[]protocol.CellView{{Pos: game.Position{X: 2, Y: 2}, Kind: game.CellEmpty}}, nil)
	b.Observe(view)
	b.knownWalls[game.Position{X: 1, Y: 1}] = true
	b.knownWalls[game.Position{X: 3, Y: 3}] = true // == MapStats wall count (2)

	actions := b.Decide(view, testRng())
	if actions[0].Kind != protocol.ActionSubmitMap {
		t.Fatalf("expected submit_map first, got %s", actions[0].Kind)
	}
	if len(actions[0].Walls) != 2 {
		t.Errorf("submit walls: want 2, got %d", len(actions[0].Walls))
	}
	if len(actions) < 2 {
		t.Fatal("submit must be followed by a turn-consuming action")
	}

	// After a failed submit the bot must never submit again.
	b.Observe(makeView(5, game.Position{X: 2, Y: 2}, nil, []protocol.Event{{
		Kind: protocol.EventMapSubmitted,
		Payload: map[string]interface{}{
			"playerName": "Bot1 (hard)", "correct": false, "wrong": 2,
		},
	}}))
	actions = b.Decide(view, testRng())
	if actions[0].Kind == protocol.ActionSubmitMap {
		t.Error("bot resubmitted after a failed submit")
	}
}

func TestHardShootsRevealedOpponent(t *testing.T) {
	b := New("bot-1", "Bot1 (hard)", Hard)
	view := makeView(5, game.Position{X: 0, Y: 2},
		[]protocol.CellView{{Pos: game.Position{X: 0, Y: 2}, Kind: game.CellEmpty}}, nil)
	view.Self.Inventory = []game.Item{{ID: "b1", Kind: game.ItemBullet}}
	view.Others = []protocol.OtherPlayerView{{ID: "p2", Name: "Alice", Alive: true}}

	// Trap publicly revealed Alice at (4,2) — same row, to the east.
	b.Observe(makeView(5, game.Position{X: 0, Y: 2}, nil, []protocol.Event{{
		Kind: protocol.EventTrapTriggered,
		Payload: map[string]interface{}{
			"effect": "reveal_position", "playerId": "p2", "playerName": "Alice",
			"pos": map[string]int{"x": 4, "y": 2},
		},
	}}))

	actions := b.Decide(view, testRng())
	last := actions[len(actions)-1]
	if last.Kind != protocol.ActionShoot || last.Direction != "E" {
		t.Errorf("expected shoot E at revealed opponent, got %s %s", last.Kind, last.Direction)
	}
}

func TestOpponentCandidateShiftOnMove(t *testing.T) {
	b := New("bot-1", "Bot1 (hard)", Hard)
	b.Observe(makeView(5, game.Position{X: 0, Y: 0}, nil, []protocol.Event{{
		Kind: protocol.EventTrapTriggered,
		Payload: map[string]interface{}{
			"effect": "reveal_position", "playerName": "Alice",
			"pos": map[string]int{"x": 2, "y": 2},
		},
	}}))
	// Alice moves right → candidates must collapse to (3,2).
	b.Observe(makeView(5, game.Position{X: 0, Y: 0}, nil, []protocol.Event{{
		Kind: protocol.EventPlayerMoved,
		Payload: map[string]interface{}{
			"playerName": "Alice", "direction": "right", "success": true, "blockType": "blank",
		},
	}}))
	set := b.candidates["Alice"]
	if len(set) != 1 || !set[game.Position{X: 3, Y: 2}] {
		t.Errorf("candidates after tracked move: want exactly {(3,2)}, got %v", set)
	}
}
