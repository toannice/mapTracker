package room

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	mrand "math/rand/v2"
	"sort"
	"strings"
	"time"

	"github.com/your-org/blindmap/internal/bot"
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
	joinOrder   []game.PlayerID // tracks join order for debug-map position assignment
	bots        map[game.PlayerID]*bot.Bot
	botSeq      int
	deleteTimer *time.Timer // pending room teardown after game_over; canceled by return_to_lobby
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
		bots:     make(map[game.PlayerID]*bot.Bot),
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
	case "bot_act":
		r.handleBotAct(ctx, msg)
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
	r.joinOrder = append(r.joinOrder, msg.PlayerID)
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

	// add_bot / remove_bot are lobby-phase actions from the host
	if data.Kind == protocol.ActionAddBot {
		r.handleAddBot(msg, &data)
		return
	}
	if data.Kind == protocol.ActionRemoveBot {
		r.handleRemoveBot(msg, &data)
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
			r.startGame(data.DebugMap, data.ControlMap, data.ControlScenario)
		}
		return
	}

	// return_to_lobby is a post-game action from any player — it works even
	// though the phase is Ended, so it must be dispatched before the
	// PhaseActive-only guard below.
	if data.Kind == protocol.ActionReturnToLobby {
		r.handleReturnToLobby(msg)
		return
	}

	if r.state.Phase != game.PhaseActive {
		return
	}

	if data.Kind == protocol.ActionPause {
		if !r.state.Paused {
			r.state.Paused = true
			remaining := r.state.TurnDeadline.UnixMilli() - time.Now().UnixMilli()
			if remaining < 0 {
				remaining = 0
			}
			r.state.PauseRemainingMs = remaining
			r.broadcastTurnResult(nil)
		}
		return
	}
	if data.Kind == protocol.ActionResume {
		if r.state.Paused {
			r.state.Paused = false
			r.state.TurnDeadline = time.Now().Add(time.Duration(r.state.PauseRemainingMs) * time.Millisecond)
			r.broadcastTurnResult(nil)
			r.scheduleBotAct()
		}
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
			r.deleteTimer = time.AfterFunc(60*time.Second, func() { r.onDelete(r.state.RoomID) })
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
		r.deleteTimer = time.AfterFunc(60*time.Second, func() { r.onDelete(r.state.RoomID) })
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
			r.hostID = ""
			for id, pl := range r.state.Players {
				if !pl.IsBot { // bots can never be host
					r.hostID = id
					break
				}
			}
		}
		// A lobby with no humans left (only bots, or empty) is dead — tear it down.
		hasHuman := false
		for _, pl := range r.state.Players {
			if !pl.IsBot {
				hasHuman = true
				break
			}
		}
		if !hasHuman {
			r.onDelete(r.state.RoomID)
			return
		}
		r.broadcastLobbyUpdate()
		return
	}

	slog.Info("player disconnected", "roomId", r.state.RoomID, "playerName", p.Name)

	// Draw when nobody can meaningfully keep playing (see gameCanContinue).
	if r.gameCanContinue() {
		return
	}
	r.state.Phase = game.PhaseEnded
	r.state.WinReason = "draw"
	r.broadcastGameOver()
	r.deleteTimer = time.AfterFunc(60*time.Second, func() { r.onDelete(r.state.RoomID) })
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

func (r *Room) startGame(debugMap bool, controlMap bool, controlScenario int) {
	seed := cryptoRandSeed()
	r.rng = mrand.New(mrand.NewPCG(seed, seed>>32))

	switch {
	case controlMap:
		scenario := controlScenario
		if scenario < 1 || scenario > 2 {
			scenario = 1
		}
		r.state.MapSize = game.ControlMapSize
		r.state.Grid = game.BuildControlMap()
		slog.Info("control map loaded", "roomId", r.state.RoomID, "scenario", scenario)

		r.state.Phase = game.PhaseActive
		r.state.Turn = 1
		r.state.TurnDeadline = time.Now().Add(time.Duration(r.state.TurnSeconds) * time.Second)

		nameToID := make(map[string]game.PlayerID, len(r.state.Players))
		for id, p := range r.state.Players {
			nameToID[p.Name] = id
		}
		controlOrder := []string{"Me", "Bot1", "Bot2"}
		for _, name := range controlOrder {
			id, ok := nameToID[name]
			if !ok {
				continue
			}
			p := r.state.Players[id]
			r.state.TurnOrder = append(r.state.TurnOrder, id)
			pos := game.ControlPositionByName(name, scenario)
			p.Pos = pos
			p.StartPos = pos
			p.VisitedCells[pos] = true
		}
		for _, id := range r.joinOrder {
			already := false
			for _, tid := range r.state.TurnOrder {
				if tid == id {
					already = true
					break
				}
			}
			if already {
				continue
			}
			p := r.state.Players[id]
			r.state.TurnOrder = append(r.state.TurnOrder, id)
			occupied := make(map[game.Position]bool)
			pos := randomFreePos(r.state.Grid, r.state.MapSize, r.rng, occupied)
			p.Pos = pos
			p.StartPos = pos
			p.VisitedCells[pos] = true
		}
		slog.Info("game started", "roomId", r.state.RoomID, "playerCount", len(r.state.Players))
		for id, c := range r.conns {
			view := protocol.BuildPlayerView(r.state, id, nil)
			r.sendEnvelope(c, "game_start", view)
		}
		r.feedBots(nil)
		r.scheduleBotAct()
		return

	case debugMap:
		r.state.MapSize = game.DebugMapSize
		r.state.Grid = game.BuildDebugMap()
		slog.Info("debug map loaded", "roomId", r.state.RoomID, "size", game.DebugMapSize)
	default:
		wallPct := randomWallPct(r.rng)
		r.state.Grid = game.GenerateMap(r.state.MapSize, r.rng, wallPct, len(r.state.Players))
		slog.Info("map generated", "roomId", r.state.RoomID, "wallPct", int(wallPct*100))
	}

	r.state.Phase = game.PhaseActive
	r.state.Turn = 1
	r.state.TurnDeadline = time.Now().Add(time.Duration(r.state.TurnSeconds) * time.Second)

	if debugMap {
		// Assign positions and turn order by name so tests are deterministic
		// regardless of goroutine scheduling / join order races.
		nameToID := make(map[string]game.PlayerID, len(r.state.Players))
		for id, p := range r.state.Players {
			nameToID[p.Name] = id
		}
		debugOrder := []string{"Alice", "Bot1", "Bot2"}
		for _, name := range debugOrder {
			id, ok := nameToID[name]
			if !ok {
				continue
			}
			p := r.state.Players[id]
			r.state.TurnOrder = append(r.state.TurnOrder, id)
			pos := game.DebugPositionByName(name)
			p.Pos = pos
			p.StartPos = pos
			p.VisitedCells[pos] = true
		}
		// Any extra players not in debugOrder get a random position at the end.
		for _, id := range r.joinOrder {
			p := r.state.Players[id]
			already := false
			for _, tid := range r.state.TurnOrder {
				if tid == id {
					already = true
					break
				}
			}
			if already {
				continue
			}
			r.state.TurnOrder = append(r.state.TurnOrder, id)
			occupied := make(map[game.Position]bool)
			pos := randomFreePos(r.state.Grid, r.state.MapSize, r.rng, occupied)
			p.Pos = pos
			p.StartPos = pos
			p.VisitedCells[pos] = true
		}
	} else {
		occupied := make(map[game.Position]bool)
		for id, p := range r.state.Players {
			r.state.TurnOrder = append(r.state.TurnOrder, id)
			pos := randomFreePos(r.state.Grid, r.state.MapSize, r.rng, occupied)
			occupied[pos] = true
			p.Pos = pos
			p.StartPos = pos
			p.VisitedCells[pos] = true
		}
	}

	slog.Info("game started", "roomId", r.state.RoomID, "playerCount", len(r.state.Players))

	for id, c := range r.conns {
		view := protocol.BuildPlayerView(r.state, id, nil)
		r.sendEnvelope(c, "game_start", view)
	}
	r.feedBots(nil)
	r.scheduleBotAct()
}

func (r *Room) checkTurnDeadline() {
	if r.state.Paused {
		return
	}
	if time.Now().Before(r.state.TurnDeadline) {
		return
	}
	currentTurn := r.state.TurnOrder[r.state.CurrentIdx%len(r.state.TurnOrder)]
	events := []protocol.Event{{
		Kind: protocol.EventTurnSkipped,
		Payload: map[string]string{
			"playerId":   string(currentTurn),
			"playerName": r.playerName(currentTurn),
			"reason":     "timeout",
		},
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
			// No separate turn_skipped broadcast here: the trap_triggered
			// event already told everyone this player would lose their next
			// turn when they stepped on it, so a second notification later
			// (often several turns on, once it's actually their turn again)
			// only repeats the same fact out of order and confuses the feed.
			p.SkipNextTurn = false
			continue
		}
		break
	}
	r.scheduleBotAct()
}

func (r *Room) broadcastTurnResult(events []protocol.Event) {
	if events == nil {
		events = []protocol.Event{}
	}
	for id, c := range r.conns {
		view := protocol.BuildPlayerView(r.state, id, events)
		r.sendEnvelope(c, "turn_result", view)
	}
	r.feedBots(events)
}

func (r *Room) broadcastGameOver() {
	var winnerName *string
	if r.state.Winner != nil {
		if p, ok := r.state.Players[*r.state.Winner]; ok {
			s := p.Name
			winnerName = &s
		}
	}
	players := make([]protocol.PlayerStartView, 0, len(r.state.Players))
	for _, p := range r.state.Players {
		players = append(players, protocol.PlayerStartView{
			Name:     p.Name,
			StartPos: p.StartPos,
			Alive:    p.Alive,
		})
	}
	sort.Slice(players, func(i, j int) bool { return players[i].Name < players[j].Name })

	data := protocol.GameOverData{
		Winner:    winnerName,
		WinReason: r.state.WinReason,
		Map:       protocol.BuildFullMapView(r.state),
		MapSize:   r.state.MapSize,
		Players:   players,
	}
	slog.Info("game ended", "roomId", r.state.RoomID, "winner", winnerName, "winReason", r.state.WinReason)
	for _, c := range r.conns {
		r.sendEnvelope(c, "game_over", data)
	}
}

// handleReturnToLobby brings the room back to PhaseLobby so a finished match
// can be followed by another one without re-joining. Any connected player may
// trigger it; the first one to do so resets the shared room state, and every
// other player's own "Quay về" click just re-syncs their client to it. Only
// meaningful post-game — canceling the pending auto-delete lets the room
// outlive its normal 60s post-game teardown while players regroup.
func (r *Room) handleReturnToLobby(msg conn.IncomingMsg) {
	if _, ok := r.state.Players[msg.PlayerID]; !ok {
		return
	}
	if r.state.Phase != game.PhaseEnded {
		// Someone already brought the room back — just resync this client.
		r.broadcastLobbyUpdate()
		return
	}

	if r.deleteTimer != nil {
		r.deleteTimer.Stop()
		r.deleteTimer = nil
	}

	r.state.Phase = game.PhaseLobby
	r.state.Grid = nil
	r.state.TurnOrder = nil
	r.state.CurrentIdx = 0
	r.state.Turn = 0
	r.state.Winner = nil
	r.state.WinReason = ""
	r.state.Paused = false
	r.state.PauseRemainingMs = 0

	for _, p := range r.state.Players {
		p.Pos = game.Position{}
		p.StartPos = game.Position{}
		p.Alive = true
		p.SkipNextTurn = false
		p.InfoBlackout = false
		p.MaxSubmit = 3
		p.Inventory = []game.Item{}
		p.VisitedCells = make(map[game.Position]bool)
	}
	for _, b := range r.bots {
		b.Reset()
	}

	slog.Info("room returned to lobby", "roomId", r.state.RoomID)
	r.broadcastLobbyUpdate()
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

// randomWallPct picks wall density biased toward 25–35%, occasionally 5–45%.
func randomWallPct(rng *mrand.Rand) float64 {
	if rng.Float64() < 0.80 {
		return 0.25 + rng.Float64()*0.10 // common: 25–35%
	}
	return 0.05 + rng.Float64()*0.40 // rare: 5–45%
}
