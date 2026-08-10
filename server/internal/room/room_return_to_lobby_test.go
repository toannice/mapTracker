package room

import (
	"context"
	"testing"
	"time"

	"github.com/your-org/blindmap/internal/game"
	"github.com/your-org/blindmap/internal/protocol"
)

// endGameAsIfElimination puts a just-finished match into the same state
// broadcastGameOver / handleAction would leave it in, without needing a full
// play-through: Phase Ended, a winner, a populated grid, and players carrying
// leftover per-match state (dead, empty inventory, visited cells).
func endGameAsIfElimination(r *Room, winner game.PlayerID) {
	r.state.Phase = game.PhaseEnded
	r.state.Winner = &winner
	r.state.WinReason = "last_alive"
}

func TestReturnToLobbyResetsRoomAndPlayers(t *testing.T) {
	ctx := context.Background()
	r := newTestRoom(nil)
	joinPlayer(r, ctx, "h1", "Host")
	joinPlayer(r, ctx, "p2", "Guest")
	sendAct(r, ctx, "h1", protocol.ActionData{Kind: protocol.ActionAddBot, Difficulty: "hard"})
	sendAct(r, ctx, "h1", protocol.ActionData{Kind: "start_game", MapSize: 5, TurnSeconds: 30})
	if r.state.Phase != game.PhaseActive {
		t.Fatal("game did not start")
	}

	// Feed the bot something to remember, then simulate the match ending with
	// one player dead and the other having drifted from their start cell.
	r.feedBots(nil)
	host := r.state.Players["h1"]
	host.Alive = false
	guest := r.state.Players["p2"]
	guest.Pos = game.Position{X: 3, Y: 3}
	guest.MaxSubmit = 1
	guest.Inventory = []game.Item{{ID: "b1", Kind: game.ItemBullet}}
	endGameAsIfElimination(r, "p2")

	sendAct(r, ctx, "p2", protocol.ActionData{Kind: protocol.ActionReturnToLobby})

	if r.state.Phase != game.PhaseLobby {
		t.Fatalf("phase after return: want lobby, got %s", r.state.Phase)
	}
	if r.state.Winner != nil || r.state.WinReason != "" {
		t.Errorf("winner/winReason not cleared: %+v %q", r.state.Winner, r.state.WinReason)
	}
	if r.state.Grid != nil || r.state.TurnOrder != nil {
		t.Error("grid/turnOrder not cleared")
	}
	if !host.Alive {
		t.Error("dead player not revived for next match")
	}
	if guest.MaxSubmit != 3 {
		t.Errorf("submitsLeft not reset: got %d", guest.MaxSubmit)
	}
	if len(guest.Inventory) != 0 {
		t.Error("inventory not cleared")
	}
	if guest.Pos != (game.Position{}) {
		t.Error("position not cleared")
	}

	// Starting a new match must work cleanly from here.
	sendAct(r, ctx, "h1", protocol.ActionData{Kind: "start_game", MapSize: 5, TurnSeconds: 30})
	if r.state.Phase != game.PhaseActive {
		t.Fatal("second match did not start after returning to lobby")
	}
}

func TestReturnToLobbyOnlyValidAfterGameEnded(t *testing.T) {
	ctx := context.Background()
	r := newTestRoom(nil)
	joinPlayer(r, ctx, "h1", "Host")
	sendAct(r, ctx, "h1", protocol.ActionData{Kind: "start_game", MapSize: 5, TurnSeconds: 30})
	if r.state.Phase != game.PhaseActive {
		t.Fatal("game did not start")
	}

	// Mid-game: return_to_lobby must be a no-op (just resyncs, doesn't reset).
	sendAct(r, ctx, "h1", protocol.ActionData{Kind: protocol.ActionReturnToLobby})
	if r.state.Phase != game.PhaseActive {
		t.Errorf("mid-game return_to_lobby must not change phase, got %s", r.state.Phase)
	}
}

func TestReturnToLobbyCancelsAutoDelete(t *testing.T) {
	ctx := context.Background()
	deleted := false
	r := newTestRoom(func(game.RoomID) { deleted = true })
	joinPlayer(r, ctx, "h1", "Host")
	sendAct(r, ctx, "h1", protocol.ActionData{Kind: "start_game", MapSize: 5, TurnSeconds: 30})
	endGameAsIfElimination(r, "h1")
	r.broadcastGameOver()
	// Stand in for the real 60s post-game teardown timer (room.go schedules
	// this after every broadcastGameOver) without actually waiting 60s.
	r.deleteTimer = time.AfterFunc(time.Hour, func() { r.onDelete(r.state.RoomID) })

	sendAct(r, ctx, "h1", protocol.ActionData{Kind: protocol.ActionReturnToLobby})

	if r.deleteTimer != nil {
		t.Error("deleteTimer should be nil after returning to lobby")
	}
	if deleted {
		t.Error("room must not be torn down once it has returned to the lobby")
	}
}
