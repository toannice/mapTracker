package room

// Deterministic golden-file replay tests.
//
// All special cells in BuildTestMap4x4 carry ForceEffect/ForcePos, so results
// are fully deterministic — no seed dependency. On every future run each replay
// must produce byte-identical output.
//
// First-time setup / after intentional behaviour changes:
//
//	go test ./internal/room/ -run TestGolden -update
//
// Three scenarios, three golden files:
//
//	TestGoldenWalkReplay   golden_walk.json       — Alice walks every event tile;
//	                                                records BOTH Alice's and Bot2's
//	                                                filtered views per step so you
//	                                                can see what each player sees.
//	TestGoldenSubmitMap    golden_submit.json     — submit wrong (costs an attempt),
//	                                                then submit correct (wins).
//	TestGoldenLastAlive    golden_lastalive.json  — Alice shoots 2 bots; last
//	                                                survivor wins.
//
// Map (X→, Y↓):
//
//	      X=0          X=1                  X=2                    X=3
//	Y=0  [Alice]       trap:reveal_pos      trap:lose_bullet       trap:lose_next_turn
//	Y=1  info:own_start reward:all_pos_rev  reward:all_bullets     reward:nuke_pending
//	Y=2  info:other_pos info:surroundings   trap:info_blackout     trap:teleport→(2,3)
//	Y=3  [Bot2 NPC]    empty                empty                  wall

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/your-org/blindmap/internal/game"
	"github.com/your-org/blindmap/internal/protocol"
)

var updateGolden = flag.Bool("update", false, "overwrite golden files with current output")

type goldenEvent struct {
	Kind         protocol.EventKind `json:"kind"`
	Private      bool               `json:"private"`
	StableFields map[string]string  `json:"stableFields,omitempty"`
}

// goldenStep is one action's worth of data. Events is the raw ground-truth
// event list. Views is the per-player filtered result of BuildPlayerView —
// i.e. exactly what each named player would receive over the wire.
type goldenStep struct {
	Turn       int                      `json:"turn"`
	PlayerID   string                   `json:"playerId"`
	ActionKind string                   `json:"actionKind"`
	Direction  string                   `json:"direction,omitempty"`
	Events     []goldenEvent            `json:"events"`
	Views      map[string][]goldenEvent `json:"views,omitempty"`
	Phase      game.Phase               `json:"phase"`
	PlayerPos  game.Position            `json:"playerPos"`
}

func stablePayloadFields(e protocol.Event) map[string]string {
	out := map[string]string{"kind": string(e.Kind)}
	switch m := e.Payload.(type) {
	case map[string]interface{}:
		for _, key := range []string{"effect", "success", "blockType", "correct", "lost", "wrong", "submitsLeft"} {
			if v, ok := m[key]; ok {
				out[key] = asStr(v)
			}
		}
	case map[string]string:
		for _, key := range []string{"effect", "playerId"} {
			if v := m[key]; v != "" {
				out[key] = v
			}
		}
	}
	return out
}

func asStr(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func toGoldenEvents(events []protocol.Event) []goldenEvent {
	out := make([]goldenEvent, len(events))
	for i, e := range events {
		out[i] = goldenEvent{
			Kind:         e.Kind,
			Private:      e.ForPlayerID != "",
			StableFields: stablePayloadFields(e),
		}
	}
	return out
}

// recordViews runs BuildPlayerView for each viewer and captures the events that
// viewer would actually receive. This is the third-person lens: private clues
// vanish for observers, acting-only fields are stripped, info_blackout collapses
// to a generic notice. Map keys are player names; JSON marshals them sorted.
func recordViews(state *game.GameState, events []protocol.Event, viewers []game.PlayerID) map[string][]goldenEvent {
	out := make(map[string][]goldenEvent, len(viewers))
	for _, vid := range viewers {
		view := protocol.BuildPlayerView(state, vid, events)
		out[string(state.Players[vid].Name)] = toGoldenEvents(view.Events)
	}
	return out
}

func compareGolden(t *testing.T, goldenPath string, steps interface{}) {
	t.Helper()
	got, err := json.MarshalIndent(steps, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("golden file written: %s (%d bytes)", goldenPath, len(got))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file missing — run once with -update to create it:\n  go test ./internal/room/ -run TestGolden -update\nerror: %v", err)
	}

	gotStr := string(got) + "\n"
	wantStr := string(want)
	if gotStr != wantStr {
		gotLines := splitLines(gotStr)
		wantLines := splitLines(wantStr)
		for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
			if wantLines[i] != gotLines[i] {
				t.Errorf("%s mismatch at line %d:\n  want: %s\n   got: %s", goldenPath, i+1, wantLines[i], gotLines[i])
				break
			}
		}
		if len(gotLines) != len(wantLines) {
			t.Errorf("%s line count: want %d, got %d", goldenPath, len(wantLines), len(gotLines))
		}
		if !t.Failed() {
			t.Errorf("%s differs (no single-line diff found)", goldenPath)
		}
	}
}

// --- Scenario 1: Alice walks every event tile; record Alice + Bot2 views. ---

func TestGoldenWalkReplay(t *testing.T) {
	aliceID := game.TestPlayerAliceID
	bot2 := game.BuildTestBot2NPC()

	alicePos := game.Position{X: 0, Y: 0}
	alice := &game.Player{
		ID:           aliceID,
		Name:         "Alice",
		Alive:        true,
		Pos:          alicePos,
		StartPos:     alicePos,
		Inventory:    []game.Item{{ID: "b1", Kind: game.ItemBullet}, {ID: "b2", Kind: game.ItemBullet}},
		MaxSubmit:    3,
		VisitedCells: map[game.Position]bool{alicePos: true},
	}

	state := &game.GameState{
		RoomID:      "GOLDEN1",
		Phase:       game.PhaseActive,
		MapSize:     4,
		TurnSeconds: 30,
		Players: map[game.PlayerID]*game.Player{
			aliceID: alice,
			bot2.ID: bot2,
		},
		Grid:      game.BuildTestMap4x4(),
		TurnOrder: []game.PlayerID{aliceID},
	}

	viewers := []game.PlayerID{aliceID, bot2.ID}

	script := []protocol.ActionData{
		{Kind: protocol.ActionMove, Direction: "E"}, // 1:  →(1,0) trap:reveal_position
		{Kind: protocol.ActionMove, Direction: "E"}, // 2:  →(2,0) trap:lose_bullet
		{Kind: protocol.ActionMove, Direction: "E"}, // 3:  →(3,0) trap:lose_next_turn
		{Kind: protocol.ActionMove, Direction: "S"}, // 4:  ↓(3,1) reward:nuke_pending [→ nuke_target]
		{Kind: protocol.ActionMove, Direction: "W"}, // 5:  ←(2,1) reward:all_bullet_locations
		{Kind: protocol.ActionMove, Direction: "W"}, // 6:  ←(1,1) reward:all_positions_revealed
		{Kind: protocol.ActionMove, Direction: "W"}, // 7:  ←(0,1) info:own_start_pos (private)
		{Kind: protocol.ActionMove, Direction: "S"}, // 8:  ↓(0,2) info:other_player_pos (private)
		{Kind: protocol.ActionMove, Direction: "E"}, // 9:  →(1,2) info:surroundings_3x3 (private)
		{Kind: protocol.ActionMove, Direction: "E"}, // 10: →(2,2) trap:info_blackout
		{Kind: protocol.ActionMove, Direction: "E"}, // 11: →(3,2) trap:teleport→(2,3)
		{Kind: protocol.ActionMove, Direction: "W"}, // 12: ←(1,3) empty
		{Kind: protocol.ActionShoot, Direction: "W"},// 13: shoot left → (0,3) Bot2 eliminated → win
	}

	steps := make([]goldenStep, 0, len(script)+1)
	for i, action := range script {
		turn := i + 1
		act := action

		if state.Paused {
			nukeAct := &protocol.ActionData{Kind: protocol.ActionNukeTarget, NukeX: 3, NukeY: 3}
			events, err := AdvanceTurn(state, nil, aliceID, nukeAct)
			if err != nil {
				t.Fatalf("turn %d nuke_target: %v", turn, err)
			}
			steps = append(steps, goldenStep{
				Turn: turn, PlayerID: string(aliceID), ActionKind: string(nukeAct.Kind),
				Events: toGoldenEvents(events), Views: recordViews(state, events, viewers),
				Phase: state.Phase, PlayerPos: alice.Pos,
			})
			if state.Phase == game.PhaseEnded {
				break
			}
		}

		events, err := AdvanceTurn(state, nil, aliceID, &act)
		if err != nil {
			t.Fatalf("turn %d (%s %s): %v", turn, act.Kind, act.Direction, err)
		}
		steps = append(steps, goldenStep{
			Turn: turn, PlayerID: string(aliceID), ActionKind: string(act.Kind), Direction: act.Direction,
			Events: toGoldenEvents(events), Views: recordViews(state, events, viewers),
			Phase: state.Phase, PlayerPos: alice.Pos,
		})
		if state.Phase == game.PhaseEnded {
			t.Logf("walk ended at turn %d: %s", turn, state.WinReason)
			break
		}
	}

	compareGolden(t, "testdata/golden_walk.json", steps)
}

// --- Scenario 2: submit_map wrong (costs attempt) then correct (wins). ---

func TestGoldenSubmitMap(t *testing.T) {
	aliceID := game.TestPlayerAliceID
	alice := &game.Player{
		ID:           aliceID,
		Name:         "Alice",
		Alive:        true,
		Pos:          game.Position{X: 0, Y: 0},
		MaxSubmit:    3,
		Inventory:    []game.Item{},
		VisitedCells: map[game.Position]bool{{X: 0, Y: 0}: true},
	}
	state := &game.GameState{
		RoomID:      "GOLDEN2",
		Phase:       game.PhaseActive,
		MapSize:     4,
		TurnSeconds: 30,
		Players:     map[game.PlayerID]*game.Player{aliceID: alice},
		Grid:        game.BuildTestMap4x4(), // real wall: (3,3)
		TurnOrder:   []game.PlayerID{aliceID},
	}

	type submitAttempt struct {
		label string
		walls []game.Position
	}
	attempts := []submitAttempt{
		// Wrong: marks (0,0) which isn't a wall AND misses the real (3,3).
		// Symmetric difference = 2 → wrong=2, MaxSubmit 3→2.
		{"wrong", []game.Position{{X: 0, Y: 0}}},
		// Correct: exactly the real wall set {(3,3)} → wrong=0 → win.
		{"correct", []game.Position{{X: 3, Y: 3}}},
	}

	steps := make([]goldenStep, 0, len(attempts))
	for i, a := range attempts {
		events, err := applySubmitMap(state, alice, a.walls)
		if err != nil {
			t.Fatalf("attempt %d (%s): %v", i+1, a.label, err)
		}
		steps = append(steps, goldenStep{
			Turn: i + 1, PlayerID: string(aliceID), ActionKind: string(protocol.ActionSubmitMap),
			Events: toGoldenEvents(events), Phase: state.Phase, PlayerPos: alice.Pos,
		})
		if state.Phase == game.PhaseEnded {
			t.Logf("submit won at attempt %d: %s", i+1, state.WinReason)
			break
		}
	}

	compareGolden(t, "testdata/golden_submit.json", steps)
}

// --- Scenario 3: Alice shoots 2 bots; last survivor wins. ---

func TestGoldenLastAlive(t *testing.T) {
	aliceID := game.TestPlayerAliceID
	bot2ID := game.PlayerID("bot2")
	bot3ID := game.PlayerID("bot3")

	mkBot := func(id game.PlayerID, name string, x, y int) *game.Player {
		pos := game.Position{X: x, Y: y}
		return &game.Player{ID: id, Name: name, Alive: true, Pos: pos, StartPos: pos,
			Inventory: []game.Item{}, VisitedCells: map[game.Position]bool{pos: true}}
	}

	alicePos := game.Position{X: 1, Y: 1}
	alice := &game.Player{
		ID: aliceID, Name: "Alice", Alive: true, Pos: alicePos, StartPos: alicePos,
		// two bullets for two shots
		Inventory:    []game.Item{{ID: "b1", Kind: game.ItemBullet}, {ID: "b2", Kind: game.ItemBullet}},
		MaxSubmit:    3,
		VisitedCells: map[game.Position]bool{alicePos: true},
	}

	// Empty 4×4 grid so bullets travel freely.
	grid := make([][]game.Cell, 4)
	for y := 0; y < 4; y++ {
		grid[y] = make([]game.Cell, 4)
		for x := 0; x < 4; x++ {
			grid[y][x] = game.Cell{Pos: game.Position{X: x, Y: y}, Kind: game.CellEmpty}
		}
	}

	state := &game.GameState{
		RoomID:      "GOLDEN3",
		Phase:       game.PhaseActive,
		MapSize:     4,
		TurnSeconds: 30,
		Players: map[game.PlayerID]*game.Player{
			aliceID: alice,
			bot2ID:  mkBot(bot2ID, "Bot2", 0, 1), // west of Alice
			bot3ID:  mkBot(bot3ID, "Bot3", 1, 3), // south of Alice
		},
		Grid:      grid,
		TurnOrder: []game.PlayerID{aliceID},
	}

	viewers := []game.PlayerID{aliceID, bot2ID, bot3ID}

	script := []protocol.ActionData{
		{Kind: protocol.ActionShoot, Direction: "W"}, // 1: kill Bot2 (3 alive → 2)
		{Kind: protocol.ActionShoot, Direction: "S"}, // 2: kill Bot3 (2 alive → 1) → win
	}

	steps := make([]goldenStep, 0, len(script))
	for i, action := range script {
		act := action
		events, err := AdvanceTurn(state, nil, aliceID, &act)
		if err != nil {
			t.Fatalf("shot %d (%s): %v", i+1, act.Direction, err)
		}
		steps = append(steps, goldenStep{
			Turn: i + 1, PlayerID: string(aliceID), ActionKind: string(act.Kind), Direction: act.Direction,
			Events: toGoldenEvents(events), Views: recordViews(state, events, viewers),
			Phase: state.Phase, PlayerPos: alice.Pos,
		})
		if state.Phase == game.PhaseEnded {
			t.Logf("last-alive win at shot %d: %s", i+1, state.WinReason)
			break
		}
	}

	compareGolden(t, "testdata/golden_lastalive.json", steps)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
