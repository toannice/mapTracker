package hub

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/your-org/blindmap/internal/config"
	"github.com/your-org/blindmap/internal/game"
	"github.com/your-org/blindmap/internal/room"
)

type Hub struct {
	rooms map[game.RoomID]*room.Room
	mu    sync.RWMutex
	cfg   *config.Config
}

func NewHub(cfg *config.Config) *Hub {
	return &Hub{
		rooms: make(map[game.RoomID]*room.Room),
		cfg:   cfg,
	}
}

func (h *Hub) Run(ctx context.Context) {
	<-ctx.Done()
}

func (h *Hub) CreateRoom() (game.RoomID, *room.Room, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.rooms) >= h.cfg.MaxRooms {
		return "", nil, errors.New("max rooms reached")
	}

	var id game.RoomID
	for {
		id = generateRoomID()
		if _, exists := h.rooms[id]; !exists {
			break
		}
	}

	r := room.NewRoom(id, h.cfg, h.deleteRoom)
	h.rooms[id] = r
	go r.Run(context.Background())
	slog.Info("room created", "roomId", id)
	return id, r, nil
}

func (h *Hub) GetOrCreateRoom(id game.RoomID) (*room.Room, bool, error) {
	h.mu.RLock()
	r, ok := h.rooms[id]
	h.mu.RUnlock()
	if ok {
		return r, false, nil
	}

	// Create with specified ID (client-specified room code)
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.rooms) >= h.cfg.MaxRooms {
		return nil, false, errors.New("max rooms reached")
	}
	r = room.NewRoom(id, h.cfg, h.deleteRoom)
	h.rooms[id] = r
	go r.Run(context.Background())
	slog.Info("room created", "roomId", id)
	return r, true, nil
}

func (h *Hub) GetRoom(id game.RoomID) (*room.Room, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	r, ok := h.rooms[id]
	return r, ok
}

func (h *Hub) deleteRoom(id game.RoomID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms, id)
	slog.Info("room destroyed", "roomId", id)
}

func generateRoomID() game.RoomID {
	b := make([]byte, 3)
	rand.Read(b)
	return game.RoomID(strings.ToUpper(hex.EncodeToString(b)))
}
