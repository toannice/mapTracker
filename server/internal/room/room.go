package room

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	mrand "math/rand/v2"
	"strings"
	"time"

	"github.com/your-org/blindmap/internal/config"
	"github.com/your-org/blindmap/internal/conn"
	"github.com/your-org/blindmap/internal/game"
	"github.com/your-org/blindmap/internal/protocol"
)

type Room struct {
	state       *game.GameState
	conns       map[game.PlayerID]*conn.Conn
	hostID      game.PlayerID
	inCh        chan conn.IncomingMsg
	cfg         *config.Config
	onDelete    func(game.RoomID)
	ticker      *time.Ticker
	rng         *mrand.Rand
	chatHistory []protocol.ChatMsgData
}

func NewRoom(id game.RoomID, cfg *config.Config, onDelete func(game.RoomID)) *Room {
	return &Room{
		state: &game.GameState{
			RoomID:      id,
			Phase:       game.PhaseLobby,
			MapSize:     cfg.MapSize,
			TurnSeconds: cfg.TurnSeconds,
			Players:     make(map[game.PlayerID]*game.Player),
		},
		conns:    make(map[game.PlayerID]*conn.Conn),
		inCh:     make(chan conn.IncomingMsg, 128),
		cfg:      cfg,
		onDelete: onDelete,
	}
}

func (r *Room) InCh() chan<- conn.IncomingMsg { return r.inCh }

func (r *Room) Run(ctx context.Context) {
	r.ticker = time.NewTicker(100 * time.Millisecond)
	defer r.ticker.Stop()

	for {
		select {
		case msg := <-r.inCh:
			r.handleMessage(ctx, msg)
		case <-r.ticker.C:
			if r.state.Phase == game.PhaseActive && len(r.state.TurnOrder) > 0 {
				r.checkTurnDeadline()
			}
		case <-ctx.Done():
			return
		}
	}
}

func (r *Room) handleMessage(ctx context.Context, msg conn.IncomingMsg) {
	switch msg.Envelope.Type {
	case "join":
		r.handleJoin(ctx, msg)
	case "action":
		r.handleAction(ctx, msg)
	case "chat":
		r.handleChat(msg)
	case "ping":
		r.handlePing(msg)
	case "leave":
		r.disconnect(msg.PlayerID)
	}
}

func (r *Room) handleJoin(ctx context.Context, msg conn.IncomingMsg) {
	var data protocol.JoinData
	if err := json.Unmarshal(msg.Envelope.Data, &data); err != nil {
		return
	}

	// Reconnect: client presents its old playerID
	if data.PlayerID != "" {
		existingID := game.PlayerID(data.PlayerID)
		if p, ok := r.state.Players[existingID]; ok {
			if c, ok2 := r.conns[msg.PlayerID]; ok2 {
				r.conns[existingID] = c
				delete(r.conns, msg.PlayerID)
			}
			p.LastSeen = time.Now()
			r.sendWelcome(existingID)
			slog.Info("player reconnected", "roomId", r.state.RoomID, "playerName", p.Name)
			return
		}
	}

	if r.state.Phase != game.PhaseLobby {
		r.sendError(msg.PlayerID, "GAME_IN_PROGRESS", "game already started")
		return
	}
	if len(r.state.Players) >= 8 {
		r.sendError(msg.PlayerID, "ROOM_FULL", "room is full")
		return
	}

	p := &game.Player{
		ID:           msg.PlayerID,
		Name:         data.PlayerName,
		Alive:        true,
		Inventory:    []game.Item{},
		MaxSubmit:    3,
		VisitedCells: make(map[game.Position]bool),
		ConnectedAt:  time.Now(),
		LastSeen:     time.Now(),
	}
	r.state.Players[msg.PlayerID] = p
	if len(r.state.Players) == 1 {
		r.hostID = msg.PlayerID
	}

	slog.Info("player joined", "roomId", r.state.RoomID, "playerName", p.Name)
	r.sendWelcome(msg.PlayerID)
	r.broadcastLobbyUpdate()
}

func (r *Room) handleAction(ctx context.Context, msg conn.IncomingMsg) {
	var data protocol.ActionData
	if err := json.Unmarshal(msg.Envelope.Data, &data); err != nil {
		return
	}

	// start_game is a lobby-phase action from the host
	if string(data.Kind) == "start_game" {
		if msg.PlayerID == r.hostID && r.state.Phase == game.PhaseLobby && len(r.state.Players) >= 1 {
			if data.MapSize >= 4 && data.MapSize <= 20 {
				r.state.MapSize = data.MapSize
			}
			if data.TurnSeconds >= 10 && data.TurnSeconds <= 120 {
				r.state.TurnSeconds = data.TurnSeconds
			}
			r.startGame()
		}
		return
	}

	if r.state.Phase != game.PhaseActive {
		return
	}

	// submit_map is resolved outside the turn flow — any player, any time,
	// and it never consumes a turn.
	if data.Kind == protocol.ActionSubmitMap {
		p, ok := r.state.Players[msg.PlayerID]
		if !ok {
			return
		}
		events, err := applySubmitMap(r.state, p, data.Walls)
		if err != nil {
			r.sendError(msg.PlayerID, errCode(err), err.Error())
			return
		}
		if r.state.Phase == game.PhaseEnded {
			r.broadcastTurnResult(events)
			r.broadcastGameOver()
			time.AfterFunc(60*time.Second, func() { r.onDelete(r.state.RoomID) })
			return
		}
		r.broadcastTurnResult(events)
		return
	}

	if len(r.state.TurnOrder) == 0 {
		return
	}

	currentTurn := r.state.TurnOrder[r.state.CurrentIdx%len(r.state.TurnOrder)]
	if msg.PlayerID != currentTurn {
		r.sendError(msg.PlayerID, "NOT_YOUR_TURN", "it is not your turn")
		return
	}

	events, err := AdvanceTurn(r.state, r.rng, msg.PlayerID, &data)
	if err != nil {
		r.sendError(msg.PlayerID, errCode(err), err.Error())
		return
	}

	if r.state.Phase == game.PhaseEnded {
		r.broadcastTurnResult(events)
		r.broadcastGameOver()
		time.AfterFunc(60*time.Second, func() { r.onDelete(r.state.RoomID) })
		return
	}

	r.advanceTurnIndex()
	r.broadcastTurnResult(events)
}

func (r *Room) handleChat(msg conn.IncomingMsg) {
	var data protocol.ChatData
	if err := json.Unmarshal(msg.Envelope.Data, &data); err != nil {
		return
	}
	text := strings.TrimSpace(data.Text)
	if text == "" || len([]rune(text)) > 200 {
		return
	}
	p, ok := r.state.Players[msg.PlayerID]
	if !ok {
		return
	}
	chatMsg := protocol.ChatMsgData{
		SenderName: p.Name,
		Ts:         time.Now().UnixMilli(),
		Text:       text,
	}
	r.chatHistory = append(r.chatHistory, chatMsg)
	if len(r.chatHistory) > 50 {
		r.chatHistory = r.chatHistory[len(r.chatHistory)-50:]
	}
	for _, c := range r.conns {
		r.sendEnvelope(c, "chat_msg", chatMsg)
	}
}

func (r *Room) handlePing(msg conn.IncomingMsg) {
	c, ok := r.conns[msg.PlayerID]
	if !ok {
		return
	}
	pong := protocol.Envelope{
		Type: "pong",
		Ts:   time.Now().UnixMilli(),
		Data: json.RawMessage(`{}`),
	}
	data, _ := json.Marshal(pong)
	c.Send(data)
}

func (r *Room) disconnect(playerID game.PlayerID) {
	p, ok := r.state.Players[playerID]
	if !ok {
		return
	}
	p.LastSeen = time.Now()
	delete(r.conns, playerID)

	if r.state.Phase == game.PhaseLobby {
		delete(r.state.Players, playerID)
		if playerID == r.hostID {
			for id := range r.state.Players {
				r.hostID = id
				break
			}
		}
		r.broadcastLobbyUpdate()
		return
	}

	slog.Info("player disconnected", "roomId", r.state.RoomID, "playerName", p.Name)

	// If all alive players are disconnected, game over (draw)
	for id, p2 := range r.state.Players {
		if p2.Alive {
			if _, connected := r.conns[id]; connected {
				return
			}
		}
	}
	r.state.Phase = game.PhaseEnded
	r.state.WinReason = "draw"
	r.broadcastGameOver()
	time.AfterFunc(60*time.Second, func() { r.onDelete(r.state.RoomID) })
}

func (r *Room) RegisterConn(playerID game.PlayerID, c *conn.Conn) {
	r.conns[playerID] = c
}

func (r *Room) RemoveConn(playerID game.PlayerID) {
	select {
	case r.inCh <- conn.IncomingMsg{PlayerID: playerID, Envelope: protocol.Envelope{Type: "leave"}}:
	default:
	}
}

func (r *Room) startGame() {
	seed := cryptoRandSeed()
	r.rng = mrand.New(mrand.NewPCG(seed, seed>>32))

	wallPct := randomWallPct(r.rng)
	r.state.Grid = game.GenerateMap(r.state.MapSize, r.rng, wallPct)
	slog.Info("map generated", "roomId", r.state.RoomID, "wallPct", int(wallPct*100))
	r.state.Phase = game.PhaseActive
	r.state.Turn = 1
	r.state.TurnDeadline = time.Now().Add(time.Duration(r.state.TurnSeconds) * time.Second)

	occupied := make(map[game.Position]bool)
	for id, p := range r.state.Players {
		r.state.TurnOrder = append(r.state.TurnOrder, id)
		pos := randomFreePos(r.state.Grid, r.state.MapSize, r.rng, occupied)
		occupied[pos] = true
		p.Pos = pos
		p.StartPos = pos
		p.VisitedCells[pos] = true
	}

	slog.Info("game started", "roomId", r.state.RoomID, "playerCount", len(r.state.Players))

	for id, c := range r.conns {
		view := protocol.BuildPlayerView(r.state, id, nil)
		r.sendEnvelope(c, "game_start", view)
	}
}

func (r *Room) checkTurnDeadline() {
	if time.Now().Before(r.state.TurnDeadline) {
		return
	}
	currentTurn := r.state.TurnOrder[r.state.CurrentIdx%len(r.state.TurnOrder)]
	events := []protocol.Event{{
		Kind:    protocol.EventTurnSkipped,
		Payload: map[string]string{"playerId": string(currentTurn), "playerName": r.playerName(currentTurn)},
	}}
	r.advanceTurnIndex()
	r.broadcastTurnResult(events)
}

func (r *Room) advanceTurnIndex() {
	for i := 0; i < len(r.state.TurnOrder); i++ {
		r.state.CurrentIdx++
		r.state.Turn++
		r.state.TurnDeadline = time.Now().Add(time.Duration(r.state.TurnSeconds) * time.Second)

		next := r.state.TurnOrder[r.state.CurrentIdx%len(r.state.TurnOrder)]
		p, ok := r.state.Players[next]
		if !ok || !p.Alive {
			continue
		}
		if p.SkipNextTurn {
			p.SkipNextTurn = false
			evs := []protocol.Event{{Kind: protocol.EventTurnSkipped, Payload: map[string]string{"playerId": string(next), "playerName": r.playerName(next)}}}
			r.broadcastTurnResult(evs)
			continue
		}
		break
	}
}

func (r *Room) broadcastTurnResult(events []protocol.Event) {
	if events == nil {
		events = []protocol.Event{}
	}
	for id, c := range r.conns {
		view := protocol.BuildPlayerView(r.state, id, events)
		r.sendEnvelope(c, "turn_result", view)
	}
}

func (r *Room) broadcastGameOver() {
	var winnerName *string
	if r.state.Winner != nil {
		if p, ok := r.state.Players[*r.state.Winner]; ok {
			s := p.Name
			winnerName = &s
		}
	}
	data := protocol.GameOverData{Winner: winnerName, WinReason: r.state.WinReason}
	slog.Info("game ended", "roomId", r.state.RoomID, "winner", winnerName, "winReason", r.state.WinReason)
	for _, c := range r.conns {
		r.sendEnvelope(c, "game_over", data)
	}
}

func (r *Room) broadcastLobbyUpdate() {
	names := make([]string, 0, len(r.state.Players))
	for _, p := range r.state.Players {
		names = append(names, p.Name)
	}
	for id, c := range r.conns {
		lobby := protocol.LobbyView{
			RoomCode: string(r.state.RoomID),
			Players:  names,
			IsHost:   id == r.hostID,
		}
		r.sendEnvelope(c, "lobby_update", lobby)
	}
}

func (r *Room) sendWelcome(playerID game.PlayerID) {
	c, ok := r.conns[playerID]
	if !ok {
		return
	}
	names := make([]string, 0, len(r.state.Players))
	for _, p := range r.state.Players {
		names = append(names, p.Name)
	}
	lobby := protocol.LobbyView{
		RoomCode: string(r.state.RoomID),
		Players:  names,
		IsHost:   playerID == r.hostID,
	}
	welcome := protocol.WelcomeData{PlayerID: string(playerID), RoomState: lobby}
	r.sendEnvelope(c, "welcome", welcome)

	// Send existing chat history so the new client sees past messages.
	if len(r.chatHistory) > 0 {
		r.sendEnvelope(c, "chat_history", protocol.ChatHistoryData{Messages: r.chatHistory})
	}

	// On reconnect during active game, send current state
	if r.state.Phase == game.PhaseActive {
		view := protocol.BuildPlayerView(r.state, playerID, nil)
		r.sendEnvelope(c, "turn_result", view)
	}
}

func (r *Room) sendError(playerID game.PlayerID, code, msg string) {
	c, ok := r.conns[playerID]
	if !ok {
		return
	}
	r.sendEnvelope(c, "error", protocol.ErrorData{Code: code, Message: msg})
}

func (r *Room) sendEnvelope(c *conn.Conn, msgType string, payload interface{}) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	env := protocol.Envelope{Type: msgType, Ts: time.Now().UnixMilli(), Data: raw}
	data, err := json.Marshal(env)
	if err != nil {
		return
	}
	c.Send(data)
}

func cryptoRandSeed() uint64 {
	b := make([]byte, 8)
	rand.Read(b)
	var n uint64
	for i, v := range b {
		n |= uint64(v) << (i * 8)
	}
	return n
}

func randomFreePos(grid [][]game.Cell, mapSize int, rng *mrand.Rand, occupied map[game.Position]bool) game.Position {
	for {
		pos := game.Position{X: rng.IntN(mapSize), Y: rng.IntN(mapSize)}
		if occupied[pos] || grid[pos.Y][pos.X].Kind == game.CellWall {
			continue
		}
		return pos
	}
}

type codeError struct {
	code string
	msg  string
}

func (e *codeError) Error() string { return e.msg }
func (e *codeError) Code() string  { return e.code }

func (r *Room) playerName(id game.PlayerID) string {
	if p, ok := r.state.Players[id]; ok {
		return p.Name
	}
	return ""
}

func errCode(err error) string {
	type coder interface{ Code() string }
	if ce, ok := err.(coder); ok {
		return ce.Code()
	}
	return "ERROR"
}

// randomWallPct picks wall density biased toward 15–25%, occasionally 5–45%.
func randomWallPct(rng *mrand.Rand) float64 {
	if rng.Float64() < 0.80 {
		return 0.15 + rng.Float64()*0.10 // common: 15–25%
	}
	return 0.05 + rng.Float64()*0.40 // rare: 5–45%
}
