package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/coder/websocket"
	"github.com/your-org/blindmap/internal/config"
	"github.com/your-org/blindmap/internal/conn"
	"github.com/your-org/blindmap/internal/game"
	"github.com/your-org/blindmap/internal/hub"
	"github.com/your-org/blindmap/internal/protocol"
)

func main() {
	cfg := config.Load()
	initLogger(cfg.LogLevel)

	h := hub.NewHub(&cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("GET /ws", wsHandler(&cfg, h))
	mux.HandleFunc("GET /debug/test-room", debugTestRoomHandler(h))
	mux.HandleFunc("GET /debug/test-start", debugTestStartHandler(h))

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go h.Run(ctx)

	go func() {
		slog.Info("server listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}

func debugTestRoomHandler(h *hub.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		swap := r.URL.Query().Get("mode") == "swap"
		roomID := game.TestRoomID
		if swap {
			roomID = game.TestRoomID2
		}

		rm, _, err := h.GetOrCreateRoom(roomID)
		if err != nil {
			http.Error(w, "server full", http.StatusServiceUnavailable)
			return
		}

		rm.SetupTestGame(swap)

		scheme := "ws"
		if r.TLS != nil {
			scheme = "wss"
		}
		host := r.Host
		mkWS := func(id game.PlayerID, name string) string {
			return fmt.Sprintf("%s://%s/ws?room=%s&name=%s&playerId=%s",
				scheme, host, roomID, name, id)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"roomCode":   string(roomID),
			"aliceId":    string(game.TestPlayerAliceID),
			"aliceWsUrl": mkWS(game.TestPlayerAliceID, "Alice"),
			"botId":      string(game.TestPlayerBotID),
			"botWsUrl":   mkWS(game.TestPlayerBotID, "Bot1"),
		})
	}
}

func debugTestStartHandler(h *hub.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roomCode := r.URL.Query().Get("room")
		rm, ok := h.GetRoom(game.RoomID(roomCode))
		if !ok {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		rm.TriggerTestGame(false)
		w.WriteHeader(http.StatusNoContent)
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func wsHandler(cfg *config.Config, h *hub.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validateOrigin(r, cfg.AllowedOrigins) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}

		roomCode := r.URL.Query().Get("room")
		if !isValidRoomCode(roomCode) {
			http.Error(w, "invalid room code", http.StatusBadRequest)
			return
		}

		playerName := strings.TrimSpace(r.URL.Query().Get("name"))
		if len(playerName) < 1 || len(playerName) > 20 {
			http.Error(w, "invalid player name", http.StatusBadRequest)
			return
		}

		opts := &websocket.AcceptOptions{InsecureSkipVerify: true}
		ws, err := websocket.Accept(w, r, opts)
		if err != nil {
			slog.Error("ws accept error", "err", err)
			return
		}

		room, _, err := h.GetOrCreateRoom(game.RoomID(roomCode))
		if err != nil {
			ws.Close(websocket.StatusTryAgainLater, "server full")
			return
		}

		playerID := game.PlayerID(newPlayerID())
		c := conn.NewConn(ws, playerName, playerID)
		room.RegisterConn(playerID, c)
		defer func() { room.RemoveConn(c.PlayerID()) }()

		connCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		joinData := protocol.JoinData{
			RoomCode:   roomCode,
			PlayerName: playerName,
			PlayerID:   r.URL.Query().Get("playerId"),
		}
		raw, _ := json.Marshal(joinData)
		room.InCh() <- conn.IncomingMsg{
			PlayerID: playerID,
			Envelope: protocol.Envelope{Type: "join", Ts: time.Now().UnixMilli(), Data: raw},
		}

		go c.WritePump(connCtx)
		c.ReadPump(connCtx, room.InCh())
	}
}

func validateOrigin(r *http.Request, allowed []string) bool {
	for _, a := range allowed {
		if a == "*" {
			return true
		}
	}
	origin := r.Header.Get("Origin")
	for _, a := range allowed {
		if strings.EqualFold(origin, strings.TrimSpace(a)) {
			return true
		}
	}
	return false
}

func isValidRoomCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, c := range code {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}

func newPlayerID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func initLogger(level string) {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})
	slog.SetDefault(slog.New(h))
}
