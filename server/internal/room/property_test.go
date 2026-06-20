package room

// Property-based / fuzzing tests.
//
// Runs hundreds of randomised games and asserts invariants that must hold
// after *every* action regardless of what the random sequence produces.
// No golden file: these tests are referees, not recorders.
//
// Run:
//   go test ./internal/room/ -run TestProperty -v -count=1

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/your-org/blindmap/internal/game"
	"github.com/your-org/blindmap/internal/protocol"
)

// ── invariant checker ────────────────────────────────────────────────────────

func assertInvariants(t *testing.T, state *game.GameState, events []protocol.Event, label string) {
	t.Helper()

	// 1. Phase only goes forward (lobby < active < ended).
	if state.Phase == game.PhaseLobby {
		t.Errorf("%s: phase reverted to lobby after action", label)
	}

	// 2. Ended game must declare a winner or reason.
	if state.Phase == game.PhaseEnded && state.WinReason == "" {
		t.Errorf("%s: phase=ended but WinReason is empty", label)
	}

	// 3. Every player's position is inside the map and not on a wall.
	for id, p := range state.Players {
		if p.Pos.X < 0 || p.Pos.X >= state.MapSize ||
			p.Pos.Y < 0 || p.Pos.Y >= state.MapSize {
			t.Errorf("%s: player %s OOB pos %+v", label, id, p.Pos)
		} else if state.Grid[p.Pos.Y][p.Pos.X].Kind == game.CellWall {
			t.Errorf("%s: player %s standing on wall at %+v", label, id, p.Pos)
		}
	}

	// 4. MaxSubmit never goes negative.
	for id, p := range state.Players {
		if p.MaxSubmit < 0 {
			t.Errorf("%s: player %s MaxSubmit=%d (negative)", label, id, p.MaxSubmit)
		}
	}

	// 5. clue_received events are always private (ForPlayerID must be set).
	for _, e := range events {
		if e.Kind == protocol.EventClueReceived && e.ForPlayerID == "" {
			t.Errorf("%s: clue_received event missing ForPlayerID — clue was broadcast publicly", label)
		}
	}

	// 6. Private events must not appear in other players' views.
	//    Also verifies the BuildPlayerView filtering path.
	for _, e := range events {
		if e.ForPlayerID == "" {
			continue // public event — fine for everyone
		}
		for viewerID := range state.Players {
			if viewerID == e.ForPlayerID {
				continue
			}
			view := protocol.BuildPlayerView(state, viewerID, events)
			for _, ve := range view.Events {
				if ve.Kind == e.Kind {
					// Crude check: same kind appearing for wrong viewer.
					// clue_received is the only private kind at the moment.
					if e.Kind == protocol.EventClueReceived {
						t.Errorf("%s: clue_received event for %s leaked to %s",
							label, e.ForPlayerID, viewerID)
					}
				}
			}
		}
	}

	// 7. bulletFull must be stripped from observer views.
	for _, e := range events {
		if e.ActingPlayerID == "" {
			continue
		}
		m, ok := e.Payload.(map[string]interface{})
		if !ok {
			continue
		}
		if _, has := m["bulletFull"]; !has {
			continue // event doesn't carry bulletFull; nothing to check
		}
		for viewerID := range state.Players {
			if viewerID == e.ActingPlayerID {
				continue // actor is allowed to see it
			}
			view := protocol.BuildPlayerView(state, viewerID, events)
			for _, ve := range view.Events {
				if ve.Kind != e.Kind {
					continue
				}
				if vm, ok2 := ve.Payload.(map[string]interface{}); ok2 {
					if _, stillHas := vm["bulletFull"]; stillHas {
						t.Errorf("%s: bulletFull not stripped for observer %s", label, viewerID)
					}
				}
			}
		}
	}

	// 8. other_player_pos clue type must never reveal position without ForPlayerID.
	for _, e := range events {
		if e.Kind != protocol.EventClueReceived {
			continue
		}
		if m, ok := e.Payload.(map[string]interface{}); ok {
			if m["type"] == "other_player_pos" && e.ForPlayerID == "" {
				t.Errorf("%s: other_player_pos clue sent without privacy guard", label)
			}
		}
	}

	// ── name → player index for elimination cross-checks ──────────────────────
	byName := make(map[string]*game.Player, len(state.Players))
	for _, p := range state.Players {
		byName[p.Name] = p
	}

	// 9. map_submitted invariants.
	for _, e := range events {
		if e.Kind != protocol.EventMapSubmitted {
			continue
		}
		m, ok := e.Payload.(map[string]interface{})
		if !ok {
			continue
		}
		correct, _ := m["correct"].(bool)
		wrong, _ := m["wrong"].(int)
		if correct {
			// A correct submission must immediately end the game as map_complete.
			if state.Phase != game.PhaseEnded {
				t.Errorf("%s: correct map_submitted but phase=%s (not ended)", label, state.Phase)
			}
			if state.WinReason != "map_complete" {
				t.Errorf("%s: correct map_submitted but WinReason=%q (want map_complete)", label, state.WinReason)
			}
			if wrong != 0 {
				t.Errorf("%s: correct map_submitted but wrong=%d (want 0)", label, wrong)
			}
		} else {
			// A wrong submission must report ≥1 mismatched cell and a non-negative
			// remaining-attempt count.
			if wrong < 1 {
				t.Errorf("%s: wrong map_submitted but wrong=%d (want ≥1)", label, wrong)
			}
			if sl, has := m["submitsLeft"].(int); has && sl < 0 {
				t.Errorf("%s: map_submitted submitsLeft=%d (negative)", label, sl)
			}
		}
	}

	// 10. nuke_fired invariants: the blast square stays fully inside the map and
	//     its side equals the map-size-derived nuke size. Every named victim is
	//     actually dead.
	for _, e := range events {
		if e.Kind != protocol.EventNukeFired {
			continue
		}
		m, ok := e.Payload.(map[string]interface{})
		if !ok {
			continue
		}
		topX, _ := m["topX"].(int)
		topY, _ := m["topY"].(int)
		side, _ := m["side"].(int)
		if side < 1 {
			t.Errorf("%s: nuke side=%d (want ≥1)", label, side)
		}
		if topX < 0 || topY < 0 || topX+side > state.MapSize || topY+side > state.MapSize {
			t.Errorf("%s: nuke square top=(%d,%d) side=%d escapes map size %d", label, topX, topY, side, state.MapSize)
		}
		if elim, ok2 := m["eliminated"].([]string); ok2 {
			for _, name := range elim {
				if victim, has := byName[name]; has && victim.Alive {
					t.Errorf("%s: nuke lists %q as eliminated but player still alive", label, name)
				}
			}
		}
	}

	// 11. player_eliminated → the named player must actually be dead in state.
	for _, e := range events {
		if e.Kind != protocol.EventPlayerEliminated {
			continue
		}
		if m, ok := e.Payload.(map[string]string); ok {
			if victim, has := byName[m["playerName"]]; has && victim.Alive {
				t.Errorf("%s: player_eliminated for %q but player still alive", label, m["playerName"])
			}
		}
	}

	// 12. portal_used → destination must be in-bounds and never a wall.
	for _, e := range events {
		if e.Kind != protocol.EventPortalUsed {
			continue
		}
		m, ok := e.Payload.(map[string]interface{})
		if !ok {
			continue
		}
		dest, ok2 := m["dest"].(game.Position)
		if !ok2 {
			continue
		}
		if dest.X < 0 || dest.X >= state.MapSize || dest.Y < 0 || dest.Y >= state.MapSize {
			t.Errorf("%s: portal dest %+v out of bounds", label, dest)
		} else if state.Grid[dest.Y][dest.X].Kind == game.CellWall {
			t.Errorf("%s: portal dest %+v is a wall", label, dest)
		}
	}

	// 13. End-state consistency: a last_alive win means exactly one survivor.
	if state.Phase == game.PhaseEnded && state.WinReason == "last_alive" {
		alive := 0
		for _, p := range state.Players {
			if p.Alive {
				alive++
			}
		}
		if alive != 1 {
			t.Errorf("%s: last_alive win but %d players alive (want 1)", label, alive)
		}
	}

	// 14. A declared winner must reference a real, and (for last_alive) living, player.
	if state.Phase == game.PhaseEnded && state.Winner != nil {
		w, has := state.Players[*state.Winner]
		if !has {
			t.Errorf("%s: winner %s not in player set", label, *state.Winner)
		} else if state.WinReason == "last_alive" && !w.Alive {
			t.Errorf("%s: last_alive winner %s is dead", label, *state.Winner)
		}
	}
}

// ── random game builder ───────────────────────────────────────────────────────

func buildPropertyState(rng *rand.Rand, mapSize, playerCount int) *game.GameState {
	grid := game.GenerateMap(mapSize, rng, 0.25, playerCount)
	players := make(map[game.PlayerID]*game.Player, playerCount)
	order := make([]game.PlayerID, playerCount)
	occupied := make(map[game.Position]bool)

	for i := 0; i < playerCount; i++ {
		id := game.PlayerID(fmt.Sprintf("prop-p%d", i))
		pos := randomFreePos(grid, mapSize, rng, occupied)
		occupied[pos] = true
		p := &game.Player{
			ID:           id,
			Name:         fmt.Sprintf("P%d", i),
			Alive:        true,
			Pos:          pos,
			StartPos:     pos,
			Inventory:    []game.Item{},
			MaxSubmit:    3,
			VisitedCells: map[game.Position]bool{pos: true},
		}
		players[id] = p
		order[i] = id
	}

	return &game.GameState{
		RoomID:      "PROP01",
		Phase:       game.PhaseActive,
		MapSize:     mapSize,
		TurnSeconds: 30,
		Players:     players,
		Grid:        grid,
		TurnOrder:   order,
	}
}

func randomPlayerAction(rng *rand.Rand, p *game.Player) *protocol.ActionData {
	dirs := []string{"N", "S", "E", "W"}
	if hasBullet(p) && rng.IntN(3) == 0 {
		return &protocol.ActionData{Kind: protocol.ActionShoot, Direction: dirs[rng.IntN(4)]}
	}
	return &protocol.ActionData{Kind: protocol.ActionMove, Direction: dirs[rng.IntN(4)]}
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestPropertyInvariants runs 500 randomised games (2–4 players, 8–16 map)
// and checks the invariant set after every single action.
func TestPropertyInvariants(t *testing.T) {
	const (
		totalGames = 500
		maxTurns   = 300 // safety valve so an immortal game doesn't hang CI
	)

	for g := 0; g < totalGames; g++ {
		seed := uint64(g)*0x9e3779b97f4a7c15 + 1
		rng := rand.New(rand.NewPCG(seed, seed^0xdeadbeefcafebabe))

		mapSize := 8 + rng.IntN(9)      // 8–16
		playerCount := 2 + rng.IntN(3)  // 2–4

		state := buildPropertyState(rng, mapSize, playerCount)

		for turn := 0; turn < maxTurns; turn++ {
			if state.Phase == game.PhaseEnded {
				break
			}
			if len(state.TurnOrder) == 0 {
				break
			}

			idx := state.CurrentIdx % len(state.TurnOrder)
			currentID := state.TurnOrder[idx]
			p, ok := state.Players[currentID]
			if !ok || !p.Alive {
				// Dead player — advance index, same as room.advanceTurnIndex.
				state.CurrentIdx++
				state.Turn++
				continue
			}

			// If a nuke is pending from a previous reward, resolve it first.
			if state.Paused && p.PendingNuke {
				nukeX := rng.IntN(state.MapSize)
				nukeY := rng.IntN(state.MapSize)
				nukeAction := &protocol.ActionData{Kind: protocol.ActionNukeTarget, NukeX: nukeX, NukeY: nukeY}
				events, err := AdvanceTurn(state, rng, currentID, nukeAction)
				label := fmt.Sprintf("game=%d turn=%d nuke", g, turn)
				if err != nil {
					t.Errorf("%s: unexpected error: %v", label, err)
				} else {
					assertInvariants(t, state, events, label)
				}
				state.CurrentIdx++
				state.Turn++
				continue
			}

			action := randomPlayerAction(rng, p)
			events, err := AdvanceTurn(state, rng, currentID, action)
			label := fmt.Sprintf("game=%d turn=%d player=%s action=%s", g, turn, currentID, action.Kind)
			if err != nil {
				// Errors like NO_BULLET are valid; still advance.
				state.CurrentIdx++
				state.Turn++
				continue
			}

			assertInvariants(t, state, events, label)

			if state.Phase == game.PhaseEnded {
				break
			}
			state.CurrentIdx++
			state.Turn++
		}

		if t.Failed() {
			// Stop early on first failure — output is already verbose enough.
			t.FailNow()
		}
	}
}

// TestPropertyMaxSubmitOnlyDecreases verifies the submit-attempt counter is
// monotonically non-increasing across a long sequence of wrong submissions.
func TestPropertyMaxSubmitOnlyDecreases(t *testing.T) {
	seed := uint64(0xc0ffee_deadbeef)
	rng := rand.New(rand.NewPCG(seed, seed>>13))
	state := buildPropertyState(rng, 10, 3)

	prev := make(map[game.PlayerID]int)
	for id, p := range state.Players {
		prev[id] = p.MaxSubmit
	}

	for round := 0; round < 300; round++ {
		if state.Phase == game.PhaseEnded {
			break
		}
		for id, p := range state.Players {
			if !p.Alive || p.MaxSubmit <= 0 {
				continue
			}
			before := p.MaxSubmit
			// Submit obviously wrong walls — out-of-bounds positions are
			// silently filtered, so the symmetric diff will be non-zero.
			_, _ = applySubmitMap(state, p, []game.Position{{X: -1, Y: -1}})
			after := p.MaxSubmit
			if after > before {
				t.Errorf("round %d player %s: MaxSubmit increased %d→%d", round, id, before, after)
			}
			if after < 0 {
				t.Errorf("round %d player %s: MaxSubmit went negative (%d)", round, id, after)
			}
			prev[id] = after
		}
	}
}

// TestPropertySubmitCorrectWins feeds each game its true wall set and asserts
// the submission wins (map_complete) and trips the end-state invariants. This
// exercises invariant 9's correct-path, which the move/shoot games never reach.
func TestPropertySubmitCorrectWins(t *testing.T) {
	const games = 200
	for g := 0; g < games; g++ {
		seed := uint64(g)*0x100000001b3 + 11
		rng := rand.New(rand.NewPCG(seed, seed^0xa5a5a5a5))
		state := buildPropertyState(rng, 8+rng.IntN(9), 2+rng.IntN(3))

		// Collect the real wall positions.
		var walls []game.Position
		for y, row := range state.Grid {
			for x, cell := range row {
				if cell.Kind == game.CellWall {
					walls = append(walls, game.Position{X: x, Y: y})
				}
			}
		}

		// Pick the first alive player as the submitter.
		var submitter *game.Player
		for _, id := range state.TurnOrder {
			if p := state.Players[id]; p.Alive {
				submitter = p
				break
			}
		}
		if submitter == nil {
			continue
		}

		events, err := applySubmitMap(state, submitter, walls)
		label := fmt.Sprintf("game=%d submit-correct", g)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", label, err)
		}
		if state.Phase != game.PhaseEnded || state.WinReason != "map_complete" {
			t.Fatalf("%s: correct submit did not win: phase=%s reason=%q", label, state.Phase, state.WinReason)
		}
		if state.Winner == nil || *state.Winner != submitter.ID {
			t.Fatalf("%s: winner not set to submitter", label)
		}
		assertInvariants(t, state, events, label)
	}
}

// TestPropertyDeadPlayerNeverOnWall is a focused invariant: eliminated players
// retain their last valid position (not moved to a wall on elimination).
func TestPropertyDeadPlayerNeverOnWall(t *testing.T) {
	const games = 200
	for g := 0; g < games; g++ {
		seed := uint64(g)*0xbadcafe + 7
		rng := rand.New(rand.NewPCG(seed, ^seed))
		state := buildPropertyState(rng, 8+rng.IntN(5), 3+rng.IntN(2))

		for turn := 0; turn < 200; turn++ {
			if state.Phase == game.PhaseEnded {
				break
			}
			if len(state.TurnOrder) == 0 {
				break
			}
			idx := state.CurrentIdx % len(state.TurnOrder)
			currentID := state.TurnOrder[idx]
			p, ok := state.Players[currentID]
			if !ok || !p.Alive {
				state.CurrentIdx++
				state.Turn++
				continue
			}

			action := randomPlayerAction(rng, p)
			_, err := AdvanceTurn(state, rng, currentID, action)
			if err != nil {
				state.CurrentIdx++
				state.Turn++
				continue
			}

			// After every action, dead players must not be on walls.
			for id, pl := range state.Players {
				if pl.Alive {
					continue
				}
				if pl.Pos.X < 0 || pl.Pos.X >= state.MapSize ||
					pl.Pos.Y < 0 || pl.Pos.Y >= state.MapSize {
					continue // shouldn't happen, caught elsewhere
				}
				if state.Grid[pl.Pos.Y][pl.Pos.X].Kind == game.CellWall {
					t.Errorf("game=%d turn=%d: dead player %s resting on wall at %+v", g, turn, id, pl.Pos)
				}
			}

			if state.Phase == game.PhaseEnded {
				break
			}
			state.CurrentIdx++
			state.Turn++
		}
	}
}
