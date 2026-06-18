package room

// Deterministic golden-file replay test.
//
// Records a fixed 12-turn game on BuildTestMap4x4 with a seeded RNG.
// On every future run the replay must produce byte-identical output.
//
// First-time setup / after intentional behaviour changes:
//   go test ./internal/room/ -run TestGoldenGameReplay -update

import (
	"encoding/json"
	"flag"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/your-org/blindmap/internal/game"
	"github.com/your-org/blindmap/internal/protocol"
)

var updateGolden = flag.Bool("update", false, "overwrite golden files with current output")

// goldenStep is one turn's worth of data written to / read from the golden file.
type goldenStep struct {
	Turn      int             `json:"turn"`
	PlayerID  string          `json:"playerId"`
	ActionKind string         `json:"actionKind"`
	Direction string          `json:"direction,omitempty"`
	Events    []goldenEvent   `json:"events"`
	Phase     game.Phase      `json:"phase"`
	// Snapshot of acting player's position after the action so we catch
	// accidental teleport or OOB regressions.
	PlayerPos game.Position   `json:"playerPos"`
}

type goldenEvent struct {
	Kind    protocol.EventKind `json:"kind"`
	// private=true means the event carries ForPlayerID, i.e. it is a private clue.
	// A regression that makes a clue public will change this flag.
	Private bool               `json:"private"`
	// Only the payload fields that are always present and stable are included;
	// position-revealing fields that vary by seed are omitted from the golden
	// comparison so the file stays readable without embedding the whole payload.
	StableFields map[string]string `json:"stableFields,omitempty"`
}

func stablePayloadFields(e protocol.Event) map[string]string {
	out := map[string]string{"kind": string(e.Kind)}
	switch m := e.Payload.(type) {
	case map[string]interface{}:
		for _, key := range []string{"effect", "success", "blockType", "correct"} {
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

func makeGoldenStep(turn int, playerID game.PlayerID, action *protocol.ActionData,
	events []protocol.Event, state *game.GameState) goldenStep {

	gevents := make([]goldenEvent, len(events))
	for i, e := range events {
		gevents[i] = goldenEvent{
			Kind:         e.Kind,
			Private:      e.ForPlayerID != "",
			StableFields: stablePayloadFields(e),
		}
	}
	return goldenStep{
		Turn:       turn,
		PlayerID:   string(playerID),
		ActionKind: string(action.Kind),
		Direction:  action.Direction,
		Events:     gevents,
		Phase:      state.Phase,
		PlayerPos:  state.Players[playerID].Pos,
	}
}

func TestGoldenGameReplay(t *testing.T) {
	// Fixed seed — must never change between runs.
	const seed = uint64(42)
	rng := rand.New(rand.NewPCG(seed, seed>>32))

	aliceID := game.TestPlayerAliceID
	bot1ID := game.TestPlayerBotID
	bot2 := game.BuildTestBot2NPC()

	mkPlayer := func(id game.PlayerID, name string, x, y int) *game.Player {
		pos := game.Position{X: x, Y: y}
		return &game.Player{
			ID:           id,
			Name:         name,
			Alive:        true,
			Pos:          pos,
			StartPos:     pos,
			Inventory:    []game.Item{},
			MaxSubmit:    3,
			VisitedCells: map[game.Position]bool{pos: true},
		}
	}

	state := &game.GameState{
		RoomID:      "GOLDEN1",
		Phase:       game.PhaseActive,
		MapSize:     4,
		TurnSeconds: 30,
		Players: map[game.PlayerID]*game.Player{
			aliceID: mkPlayer(aliceID, "Alice", 0, 0),
			bot1ID:  mkPlayer(bot1ID, "Bot1", 1, 3),
			bot2.ID: bot2,
		},
		Grid:      game.BuildTestMap4x4(),
		TurnOrder: []game.PlayerID{aliceID, bot1ID},
	}

	// 12-step script:
	//   Map layout (X, Y):
	//     (0,0)Alice  (1,0)trap   (2,0)trap   (3,0)trap
	//     (0,1)info   (1,1)reward (2,1)reward (3,1)reward
	//     (0,2)info   (1,2)info   (2,2)empty  (3,2)empty
	//     (0,3)Bot2   (1,3)Bot1   (2,3)bullet (3,3)wall
	script := []struct {
		id     game.PlayerID
		action protocol.ActionData
	}{
		{aliceID, protocol.ActionData{Kind: protocol.ActionMove, Direction: "S"}}, // 1: Alice→(0,1) info
		{bot1ID,  protocol.ActionData{Kind: protocol.ActionMove, Direction: "W"}}, // 2: Bot1→(0,3) Bot2's cell
		{aliceID, protocol.ActionData{Kind: protocol.ActionMove, Direction: "S"}}, // 3: Alice→(0,2) info
		{bot1ID,  protocol.ActionData{Kind: protocol.ActionMove, Direction: "E"}}, // 4: Bot1→(1,3)
		{aliceID, protocol.ActionData{Kind: protocol.ActionMove, Direction: "E"}}, // 5: Alice→(1,2) info
		{bot1ID,  protocol.ActionData{Kind: protocol.ActionMove, Direction: "E"}}, // 6: Bot1→(2,3) bullet pickup
		{aliceID, protocol.ActionData{Kind: protocol.ActionMove, Direction: "N"}}, // 7: Alice→(1,1) reward
		{bot1ID,  protocol.ActionData{Kind: protocol.ActionShoot, Direction: "W"}},// 8: Bot1 shoots W — hits Bot2 at (0,3)
		{aliceID, protocol.ActionData{Kind: protocol.ActionMove, Direction: "N"}}, // 9: Alice→(1,0) trap
		{bot1ID,  protocol.ActionData{Kind: protocol.ActionMove, Direction: "N"}}, //10: Bot1→(2,2)
		{aliceID, protocol.ActionData{Kind: protocol.ActionMove, Direction: "N"}}, //11: Alice moves N from Y=0 → OOB, fails
		{bot1ID,  protocol.ActionData{Kind: protocol.ActionMove, Direction: "N"}}, //12: Bot1→(2,1) reward
	}

	steps := make([]goldenStep, 0, len(script))
	for i, s := range script {
		action := s.action // copy to avoid aliasing
		p, ok := state.Players[s.id]
		if !ok || !p.Alive {
			t.Logf("step %d: player %s dead/missing, skipping", i+1, s.id)
			continue
		}

		// If a nuke is pending the turn is paused; send nuke_target first.
		if state.Paused {
			nukeAction := &protocol.ActionData{Kind: protocol.ActionNukeTarget, NukeX: 0, NukeY: 0}
			events, err := AdvanceTurn(state, rng, s.id, nukeAction)
			if err != nil {
				t.Fatalf("step %d nuke_target: %v", i+1, err)
			}
			steps = append(steps, makeGoldenStep(i+1, s.id, nukeAction, events, state))
			if state.Phase == game.PhaseEnded {
				t.Logf("game ended at step %d (nuke): %s", i+1, state.WinReason)
				break
			}
		}

		events, err := AdvanceTurn(state, rng, s.id, &action)
		if err != nil {
			t.Fatalf("step %d (%s %s %s): %v", i+1, s.id, s.action.Kind, s.action.Direction, err)
		}
		steps = append(steps, makeGoldenStep(i+1, s.id, &action, events, state))

		if state.Phase == game.PhaseEnded {
			t.Logf("game ended at step %d: %s", i+1, state.WinReason)
			break
		}
	}

	got, err := json.MarshalIndent(steps, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	const goldenPath = "testdata/golden_game.json"

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
		t.Fatalf("golden file missing — run once with -update to create it:\n  go test ./internal/room/ -run TestGoldenGameReplay -update\nerror: %v", err)
	}

	// Normalise trailing newline so editors don't break the comparison.
	gotStr := string(got) + "\n"
	wantStr := string(want)
	if gotStr != wantStr {
		// Print a compact diff: first mismatched line.
		gotLines := splitLines(gotStr)
		wantLines := splitLines(wantStr)
		for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
			if wantLines[i] != gotLines[i] {
				t.Errorf("golden mismatch at line %d:\n  want: %s\n   got: %s", i+1, wantLines[i], gotLines[i])
				break
			}
		}
		if len(gotLines) != len(wantLines) {
			t.Errorf("golden line count: want %d, got %d", len(wantLines), len(gotLines))
		}
		if !t.Failed() {
			t.Errorf("golden files differ (no single-line diff found — trailing content?)")
		}
	}
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
