package room

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/your-org/blindmap/internal/config"
	"github.com/your-org/blindmap/internal/conn"
	"github.com/your-org/blindmap/internal/game"
	"github.com/your-org/blindmap/internal/protocol"
)

func newTestRoom(onDelete func(game.RoomID)) *Room {
	cfg := &config.Config{MapSize: 6, TurnSeconds: 30, MaxRooms: 10}
	if onDelete == nil {
		onDelete = func(game.RoomID) {}
	}
	return NewRoom("BOTTST", cfg, onDelete)
}

func joinPlayer(r *Room, ctx context.Context, id, name string) {
	data, _ := json.Marshal(protocol.JoinData{PlayerName: name})
	r.handleJoin(ctx, conn.IncomingMsg{
		PlayerID: game.PlayerID(id),
		Envelope: protocol.Envelope{Type: "join", Data: data},
	})
}

func sendAct(r *Room, ctx context.Context, id string, a protocol.ActionData) {
	raw, _ := json.Marshal(a)
	r.handleAction(ctx, conn.IncomingMsg{
		PlayerID: game.PlayerID(id),
		Envelope: protocol.Envelope{Type: "action", Data: raw},
	})
}

func TestAddRemoveBotHostOnly(t *testing.T) {
	ctx := context.Background()
	r := newTestRoom(nil)
	joinPlayer(r, ctx, "h1", "Host")
	joinPlayer(r, ctx, "p2", "Guest")

	// Non-host cannot add a bot.
	sendAct(r, ctx, "p2", protocol.ActionData{Kind: protocol.ActionAddBot, Difficulty: "easy"})
	if len(r.bots) != 0 {
		t.Fatal("non-host managed to add a bot")
	}

	sendAct(r, ctx, "h1", protocol.ActionData{Kind: protocol.ActionAddBot, Difficulty: "hard"})
	sendAct(r, ctx, "h1", protocol.ActionData{Kind: protocol.ActionAddBot, Difficulty: ""}) // default medium
	if len(r.bots) != 2 || len(r.state.Players) != 4 {
		t.Fatalf("want 2 bots / 4 players, got %d / %d", len(r.bots), len(r.state.Players))
	}

	// Invalid difficulty is rejected.
	sendAct(r, ctx, "h1", protocol.ActionData{Kind: protocol.ActionAddBot, Difficulty: "impossible"})
	if len(r.bots) != 2 {
		t.Fatal("invalid difficulty was accepted")
	}

	// Remove last added bot.
	sendAct(r, ctx, "h1", protocol.ActionData{Kind: protocol.ActionRemoveBot})
	if len(r.bots) != 1 || len(r.state.Players) != 3 {
		t.Fatalf("after remove: want 1 bot / 3 players, got %d / %d", len(r.bots), len(r.state.Players))
	}
}

func TestBotsPlayFullGame(t *testing.T) {
	ctx := context.Background()
	r := newTestRoom(nil)
	joinPlayer(r, ctx, "h1", "Host")
	sendAct(r, ctx, "h1", protocol.ActionData{Kind: protocol.ActionAddBot, Difficulty: "hard"})
	sendAct(r, ctx, "h1", protocol.ActionData{Kind: protocol.ActionAddBot, Difficulty: "easy"})
	sendAct(r, ctx, "h1", protocol.ActionData{Kind: "start_game", MapSize: 6, TurnSeconds: 30})
	if r.state.Phase != game.PhaseActive {
		t.Fatal("game did not start")
	}

	for i := 0; i < 300 && r.state.Phase == game.PhaseActive; i++ {
		current := r.state.TurnOrder[r.state.CurrentIdx%len(r.state.TurnOrder)]
		if _, isBot := r.bots[current]; isBot {
			r.handleBotAct(ctx, conn.IncomingMsg{
				PlayerID: current,
				Envelope: protocol.Envelope{Type: "bot_act"},
			})
		} else {
			sendAct(r, ctx, string(current), protocol.ActionData{Kind: protocol.ActionPass})
		}
	}

	// Either a bot ended the game (shot / map win) or 300 turns elapsed without
	// a stall — both prove bots act through the normal pipeline.
	if r.state.Phase == game.PhaseActive && r.state.Turn < 100 {
		t.Errorf("turns stalled: turn=%d after 300 iterations", r.state.Turn)
	}
}

func TestLobbyTeardownWhenOnlyBotsRemain(t *testing.T) {
	ctx := context.Background()
	deleted := false
	r := newTestRoom(func(game.RoomID) { deleted = true })
	joinPlayer(r, ctx, "h1", "Host")
	sendAct(r, ctx, "h1", protocol.ActionData{Kind: protocol.ActionAddBot, Difficulty: "medium"})

	r.disconnect("h1")
	if !deleted {
		t.Error("room with only bots left in lobby was not torn down")
	}
	if r.hostID == "h1" {
		t.Error("departed player still host")
	}
}
