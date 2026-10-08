package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	"github.com/your-org/blindmap/internal/release"
)

func main() {
	cfg := config.Load()
	initLogger(cfg.LogLevel)

	// Nil when GITHUB_REPO is unset, in which case the static
	// LATEST_VERSION_CODE / UPDATE_URL settings are used instead.
	tracker := release.NewTracker(cfg.GitHubRepo, time.Duration(cfg.ReleasePollMinutes)*time.Minute)
	if tracker != nil {
		cfg.Releases = tracker
	}

	h := hub.NewHub(&cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("GET /ws", wsHandler(&cfg, h))

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go h.Run(ctx)
	go tracker.Run(ctx) // no-op on a nil tracker

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
		defer room.RemoveConn(c)

		connCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		joinData := protocol.JoinData{
			RoomCode:       roomCode,
			PlayerName:     playerName,
			PlayerID:       r.URL.Query().Get("playerId"),
			ReconnectToken: r.URL.Query().Get("token"),
		}
		raw, _ := json.Marshal(joinData)
		// The room registers the socket itself if it accepts the join; this
		// goroutine must never touch room state directly.
		room.InCh() <- conn.IncomingMsg{
			PlayerID: playerID,
			Conn:     c,
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
