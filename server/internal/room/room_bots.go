package room

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/your-org/blindmap/internal/bot"
	"github.com/your-org/blindmap/internal/conn"
	"github.com/your-org/blindmap/internal/game"
	"github.com/your-org/blindmap/internal/protocol"
)

// handleAddBot adds a fair-mode bot player to the lobby. Host only.
func (r *Room) handleAddBot(msg conn.IncomingMsg, data *protocol.ActionData) {
	if r.state.Phase != game.PhaseLobby || msg.PlayerID != r.hostID {
		return
	}
	if len(r.state.Players) >= 8 {
		r.sendError(msg.PlayerID, "ROOM_FULL", "room is full")
		return
	}
	diff, ok := bot.ParseDifficulty(data.Difficulty)
	if !ok {
		r.sendError(msg.PlayerID, "BAD_DIFFICULTY", "difficulty must be easy, medium or hard")
		return
	}

	r.botSeq++
	id := game.PlayerID(fmt.Sprintf("bot-%d", r.botSeq))
	name := fmt.Sprintf("Bot%d (%s)", r.botSeq, diff)
	r.state.Players[id] = &game.Player{
		ID:           id,
		Name:         name,
		IsBot:        true,
		Alive:        true,
		Inventory:    []game.Item{},
		MaxSubmit:    3,
		VisitedCells: make(map[game.Position]bool),
		ConnectedAt:  time.Now(),
		LastSeen:     time.Now(),
	}
	r.joinOrder = append(r.joinOrder, id)
	r.bots[id] = bot.New(id, name, diff)

	slog.Info("bot added", "roomId", r.state.RoomID, "botName", name)
	r.broadcastLobbyUpdate()
}

// handleRemoveBot removes a bot by id or name; empty selector removes the most
// recently added bot. Host only, lobby only.
func (r *Room) handleRemoveBot(msg conn.IncomingMsg, data *protocol.ActionData) {
	if r.state.Phase != game.PhaseLobby || msg.PlayerID != r.hostID {
		return
	}
	var target game.PlayerID
	if data.BotID != "" {
		for id := range r.bots {
			if string(id) == data.BotID || r.playerName(id) == data.BotID {
				target = id
				break
			}
		}
	} else {
		for i := len(r.joinOrder) - 1; i >= 0; i-- {
			if _, ok := r.bots[r.joinOrder[i]]; ok {
				target = r.joinOrder[i]
				break
			}
		}
	}
	if target == "" {
		return
	}

	delete(r.bots, target)
	delete(r.state.Players, target)
	for i, id := range r.joinOrder {
		if id == target {
			r.joinOrder = append(r.joinOrder[:i], r.joinOrder[i+1:]...)
			break
		}
	}
	slog.Info("bot removed", "roomId", r.state.RoomID, "botId", target)
	r.broadcastLobbyUpdate()
}

// feedBots delivers the same filtered PlayerView a client would receive, so
// bots learn exactly what a human at the keyboard would.
func (r *Room) feedBots(events []protocol.Event) {
	for id, b := range r.bots {
		view := protocol.BuildPlayerView(r.state, id, events)
		b.Observe(&view)
	}
}

// scheduleBotAct arms a delayed self-message when the current turn belongs to
// a bot. The delay keeps the game readable for humans. The timer goroutine
// only sends on inCh — all state access stays inside the room goroutine.
func (r *Room) scheduleBotAct() {
	if r.state.Phase != game.PhaseActive || r.state.Paused || len(r.state.TurnOrder) == 0 {
		return
	}
	current := r.state.TurnOrder[r.state.CurrentIdx%len(r.state.TurnOrder)]
	if _, ok := r.bots[current]; !ok {
		return
	}
	delay := 1200*time.Millisecond + time.Duration(r.rng.IntN(800))*time.Millisecond
	time.AfterFunc(delay, func() {
		select {
		case r.inCh <- conn.IncomingMsg{PlayerID: current, Envelope: protocol.Envelope{Type: "bot_act"}}:
		default: // room channel full or gone — turn timeout will cover us
		}
	})
}

// handleBotAct runs inside the room goroutine when a bot's think-delay fires.
// Stale or duplicate timers are harmless: every check re-validates live state,
// and the resulting actions flow through the exact same handleAction path as a
// human client's.
func (r *Room) handleBotAct(ctx context.Context, msg conn.IncomingMsg) {
	if r.state.Phase != game.PhaseActive || len(r.state.TurnOrder) == 0 {
		return
	}
	if r.state.Paused {
		return // the resume handler reschedules
	}
	current := r.state.TurnOrder[r.state.CurrentIdx%len(r.state.TurnOrder)]
	if current != msg.PlayerID {
		return
	}
	b, ok := r.bots[msg.PlayerID]
	if !ok {
		return
	}

	view := protocol.BuildPlayerView(r.state, msg.PlayerID, nil)
	for _, a := range b.Decide(&view, r.rng) {
		raw, err := json.Marshal(a)
		if err != nil {
			continue
		}
		r.handleAction(ctx, conn.IncomingMsg{
			PlayerID: msg.PlayerID,
			Envelope: protocol.Envelope{Type: "action", Data: raw},
		})
		if r.state.Phase != game.PhaseActive {
			return
		}
	}
}

// gameCanContinue reports whether an active game still has a future: an alive
// connected human can act, or alive bots can act while at least one human is
// still connected to watch. Otherwise the game should end in a draw.
func (r *Room) gameCanContinue() bool {
	connectedHuman := false
	aliveBot := false
	for id, p := range r.state.Players {
		if p.IsBot {
			if p.Alive {
				aliveBot = true
			}
			continue
		}
		if _, connected := r.conns[id]; connected {
			connectedHuman = true
			if p.Alive {
				return true
			}
		}
	}
	return aliveBot && connectedHuman
}
