package conn

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/your-org/blindmap/internal/game"
	"github.com/your-org/blindmap/internal/protocol"
)

type IncomingMsg struct {
	PlayerID game.PlayerID
	Envelope protocol.Envelope
}

type Conn struct {
	wsConn     *websocket.Conn
	playerName string
	playerID   game.PlayerID
	writeCh    chan []byte
	mu         sync.Mutex
}

func NewConn(ws *websocket.Conn, playerName string, playerID game.PlayerID) *Conn {
	return &Conn{
		wsConn:     ws,
		playerName: playerName,
		playerID:   playerID,
		writeCh:    make(chan []byte, 64),
	}
}

func (c *Conn) ReadPump(ctx context.Context, roomCh chan<- IncomingMsg) {
	defer c.wsConn.CloseNow()

	var (
		count     int
		windowEnd = time.Now().Add(time.Second)
	)

	for {
		_, data, err := c.wsConn.Read(ctx)
		if err != nil {
			return
		}

		now := time.Now()
		if now.After(windowEnd) {
			count = 0
			windowEnd = now.Add(time.Second)
		}
		count++
		if count > 10 {
			slog.Warn("rate limit exceeded", "playerName", c.playerName)
			c.wsConn.Close(websocket.StatusPolicyViolation, "rate limit exceeded")
			return
		}

		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}

		select {
		case roomCh <- IncomingMsg{PlayerID: c.playerID, Envelope: env}:
		case <-ctx.Done():
			return
		}
	}
}

func (c *Conn) WritePump(ctx context.Context) {
	for {
		select {
		case data, ok := <-c.writeCh:
			if !ok {
				return
			}
			if err := c.wsConn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *Conn) Send(data []byte) {
	select {
	case c.writeCh <- data:
	default:
		// drop if buffer full
	}
}

func (c *Conn) Close() {
	c.wsConn.CloseNow()
}

// PlayerID returns the current effective player ID for this connection.
// It may be updated via SetPlayerID during a reconnect.
func (c *Conn) PlayerID() game.PlayerID { return c.playerID }

// SetPlayerID updates the player ID used by ReadPump when routing messages.
// Must be called before ReadPump starts (i.e. from the room goroutine while
// processing the join message, before the WS handler launches ReadPump).
func (c *Conn) SetPlayerID(id game.PlayerID) { c.playerID = id }
