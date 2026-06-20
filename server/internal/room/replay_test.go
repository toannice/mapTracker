package room

// Deterministic golden-file replay test.
//
// Records a fixed 15-move solo game on BuildTestMap4x4.
// All special cells have ForceEffect/ForcePos so results are fully
// deterministic — no seed dependency. On every future run the replay
// must produce byte-identical output.
//
// First-time setup / after intentional behaviour changes:
//
//	go test ./internal/room/ -run TestGoldenGameReplay -update
//
// Map (X→, Y↓):
//
//	      X=0          X=1                  X=2                    X=3
//	Y=0  [Alice]       trap:reveal_pos      trap:lose_bullet       trap:lose_next_turn
//	Y=1  info:own_start reward:all_pos_rev  reward:all_bullets     reward:nuke_pending
//	Y=2  info:other_pos info:surroundings   trap:info_blackout     trap:teleport→(2,3)
//	Y=3  [Bot2 NPC]    empty                empty                  empty
//
// Path (Alice solo, →=right ←=left ↑=up ↓=down):
//
//	Lượt | Ai    | Action          | Kết quả
//	  1  | Alice | → move phải     | (0,0)→(1,0) trap:reveal_position — lộ vị trí Alice cho tất cả
//	  2  | Alice | → move phải     | (1,0)→(2,0) trap:lose_bullet — mất 1 đạn (lost=true, còn 1)
//	  3  | Alice | → move phải     | (2,0)→(3,0) trap:lose_next_turn — set skip flag
//	  4  | Alice | ↓ move xuống    | (3,0)→(3,1) reward:all_positions_revealed — lộ vị trí tất cả
//	  5  | Alice | ← move trái     | (3,1)→(2,1) reward:all_bullet_locations — lộ vị trí tất cả ô đạn
//	  6  | Alice | ← move trái     | (2,1)→(1,1) reward:nuke_pending — game tạm dừng, Alice chọn mục tiêu
//	 6b  |       | nuke_target(3,3)| nuke bắn vào (3,3) empty — không ai chết
//	  7  | Alice | ← move trái     | (1,1)→(0,1) info:own_start_pos — clue riêng tư: vị trí xuất phát
//	  8  | Alice | ↓ move xuống    | (0,1)→(0,2) info:other_player_pos — clue riêng tư: vị trí Bot2
//	  9  | Alice | → move phải     | (0,2)→(1,2) info:surroundings_3x3 — clue riêng tư: ô xung quanh
//	 10  | Alice | → move phải     | (1,2)→(2,2) trap:info_blackout — mất quyền dùng info lượt sau
//	 11  | Alice | → move phải     | (2,2)→(3,2) trap:random_teleport → dịch chuyển đến (2,3)
//	 12  | Alice | ↓ move xuống    | (2,3)→OOB — thất bại, ở lại (2,3)
//	 13  | Alice | ← move trái     | (2,3)→(1,3) empty — thành công
//	 14  | Alice | ↓ move xuống    | (1,3)→OOB — thất bại, ở lại (1,3)
//	 15  | Alice | bắn trái        | tia đạn (0,3) trúng Bot2 → bot_bị_loại

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

type goldenStep struct {
	Turn       int           `json:"turn"`
	PlayerID   string        `json:"playerId"`
	ActionKind string        `json:"actionKind"`
	Direction  string        `json:"direction,omitempty"`
	Events     []goldenEvent `json:"events"`
	Phase      game.Phase    `json:"phase"`
	PlayerPos  game.Position `json:"playerPos"`
}

type goldenEvent struct {
	Kind         protocol.EventKind `json:"kind"`
	Private      bool               `json:"private"`
	StableFields map[string]string  `json:"stableFields,omitempty"`
}

func stablePayloadFields(e protocol.Event) map[string]string {
	out := map[string]string{"kind": string(e.Kind)}
	switch m := e.Payload.(type) {
	case map[string]interface{}:
		for _, key := range []string{"effect", "success", "blockType", "correct", "lost"} {
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

	// 15-step script — Alice solo.
	// See path table in the file header above.
	script := []protocol.ActionData{
		{Kind: protocol.ActionMove, Direction: "E"}, // 1:  →(1,0) trap:reveal_position
		{Kind: protocol.ActionMove, Direction: "E"}, // 2:  →(2,0) trap:lose_bullet
		{Kind: protocol.ActionMove, Direction: "E"}, // 3:  →(3,0) trap:lose_next_turn
		{Kind: protocol.ActionMove, Direction: "S"}, // 4:  ↓(3,1) reward:all_positions_revealed
		{Kind: protocol.ActionMove, Direction: "W"}, // 5:  ←(2,1) reward:all_bullet_locations
		{Kind: protocol.ActionMove, Direction: "W"}, // 6:  ←(1,1) reward:nuke_pending [→ nuke_target 6b]
		{Kind: protocol.ActionMove, Direction: "W"}, // 7:  ←(0,1) info:own_start_pos
		{Kind: protocol.ActionMove, Direction: "S"}, // 8:  ↓(0,2) info:other_player_pos
		{Kind: protocol.ActionMove, Direction: "E"}, // 9:  →(1,2) info:surroundings_3x3
		{Kind: protocol.ActionMove, Direction: "E"}, // 10: →(2,2) trap:info_blackout
		{Kind: protocol.ActionMove, Direction: "E"}, // 11: →(3,2) trap:teleport→(2,3)
		{Kind: protocol.ActionMove, Direction: "S"}, // 12: ↓ OOB — thất bại, ở lại (2,3)
		{Kind: protocol.ActionMove, Direction: "W"}, // 13: ←(1,3) empty
		{Kind: protocol.ActionMove, Direction: "S"}, // 14: ↓ OOB — thất bại, ở lại (1,3)
		{Kind: protocol.ActionShoot, Direction: "W"},// 15: bắn trái → (0,3) Bot2 bị loại
	}

	steps := make([]goldenStep, 0, len(script)+1) // +1 for possible nuke_target
	for i, action := range script {
		turn := i + 1
		act := action

		// After nuke_pending the game pauses; resolve with nuke_target before continuing.
		if state.Paused {
			nukeAct := &protocol.ActionData{Kind: protocol.ActionNukeTarget, NukeX: 3, NukeY: 3}
			events, err := AdvanceTurn(state, nil, aliceID, nukeAct)
			if err != nil {
				t.Fatalf("turn %d nuke_target: %v", turn, err)
			}
			steps = append(steps, makeGoldenStep(turn, aliceID, nukeAct, events, state))
			if state.Phase == game.PhaseEnded {
				t.Logf("game ended at turn %d (nuke): %s", turn, state.WinReason)
				break
			}
		}

		events, err := AdvanceTurn(state, nil, aliceID, &act)
		if err != nil {
			t.Fatalf("turn %d (%s %s): %v", turn, act.Kind, act.Direction, err)
		}
		steps = append(steps, makeGoldenStep(turn, aliceID, &act, events, state))

		if state.Phase == game.PhaseEnded {
			t.Logf("game ended at turn %d: %s", turn, state.WinReason)
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

	gotStr := string(got) + "\n"
	wantStr := string(want)
	if gotStr != wantStr {
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
