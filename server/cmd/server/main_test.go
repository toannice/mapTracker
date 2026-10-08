package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/your-org/blindmap/internal/config"
	"github.com/your-org/blindmap/internal/hub"
	"github.com/your-org/blindmap/internal/protocol"
)

// These tests drive the real /ws handler over real sockets, because the bugs
// they guard against live in the wiring between the HTTP handler, the
// connection pumps and the room goroutine. Run them with -race.

const awaitTimeout = 2 * time.Second

func startServer(t *testing.T) string {
	t.Helper()
	cfg := config.Config{MaxRooms: 10, TurnSeconds: 30, MapSize: 6, AllowedOrigins: []string{"*"}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws", wsHandler(&cfg, hub.NewHub(&cfg)))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
}

type testClient struct {
	t  *testing.T
	ws *websocket.Conn
}

// dial opens a socket to room as name. extra carries reconnect credentials.
func dial(t *testing.T, base, room, name string, extra url.Values) *testClient {
	t.Helper()
	q := url.Values{"room": {room}, "name": {name}}
	for k, v := range extra {
		q[k] = v
	}
	ws, _, err := websocket.Dial(context.Background(), base+"?"+q.Encode(), nil)
	if err != nil {
		t.Fatalf("dial %s: %v", name, err)
	}
	t.Cleanup(func() { ws.CloseNow() })
	return &testClient{t: t, ws: ws}
}

func (c *testClient) send(typ string, data any) {
	c.t.Helper()
	raw, _ := json.Marshal(data)
	env, _ := json.Marshal(protocol.Envelope{Type: typ, Data: raw})
	if err := c.ws.Write(context.Background(), websocket.MessageText, env); err != nil {
		c.t.Fatalf("send %s: %v", typ, err)
	}
}

// next returns the next message of type typ, skipping others, or false if
// none arrives within d.
func (c *testClient) next(typ string, d time.Duration) (json.RawMessage, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	for {
		_, b, err := c.ws.Read(ctx)
		if err != nil {
			return nil, false
		}
		var env protocol.Envelope
		if json.Unmarshal(b, &env) == nil && env.Type == typ {
			return env.Data, true
		}
	}
}

// first returns whatever message arrives next.
func (c *testClient) first() protocol.Envelope {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), awaitTimeout)
	defer cancel()
	_, b, err := c.ws.Read(ctx)
	if err != nil {
		c.t.Fatalf("no message within %s: %v", awaitTimeout, err)
	}
	var env protocol.Envelope
	if err := json.Unmarshal(b, &env); err != nil {
		c.t.Fatalf("decode envelope %s: %v", b, err)
	}
	return env
}

func (c *testClient) await(typ string) json.RawMessage {
	c.t.Helper()
	data, ok := c.next(typ, awaitTimeout)
	if !ok {
		c.t.Fatalf("no %q message within %s", typ, awaitTimeout)
	}
	return data
}

type welcome struct {
	PlayerID       string `json:"playerId"`
	ReconnectToken string `json:"reconnectToken"`
}

type view struct {
	CurrentTurn string `json:"currentTurn"`
	Turn        int    `json:"turn"`
	Self        struct {
		ID string `json:"id"`
	} `json:"self"`
}

func decode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return v
}

type player struct {
	*testClient
	name string
	welcome
}

// startTwoPlayerGame joins Alice (host) and Bob to room and starts a match.
// It returns both players and the game_start view everyone shares.
func startTwoPlayerGame(t *testing.T, base, room string) (alice, bob *player, start view) {
	t.Helper()
	alice = &player{testClient: dial(t, base, room, "Alice", nil), name: "Alice"}
	alice.welcome = decode[welcome](t, alice.await("welcome"))
	bob = &player{testClient: dial(t, base, room, "Bob", nil), name: "Bob"}
	bob.welcome = decode[welcome](t, bob.await("welcome"))

	alice.send("action", protocol.ActionData{Kind: "start_game", MapSize: 6})
	start = decode[view](t, alice.await("game_start"))
	bob.await("game_start")
	return alice, bob, start
}

func reconnectQuery(playerID, token string) url.Values {
	return url.Values{"playerId": {playerID}, "token": {token}}
}

func moveN(c *testClient) {
	c.send("action", protocol.ActionData{Kind: protocol.ActionMove, Direction: "N"})
}

// Joining must not touch room state from the HTTP handler goroutine. Under
// -race this fails if a socket is registered outside the room goroutine.
func TestJoinsAreRaceFree(t *testing.T) {
	base := startServer(t)
	for _, name := range []string{"Alice", "Bob", "Carol"} {
		dial(t, base, "RACE01", name, nil).await("welcome")
	}
}

// A socket whose join is refused belongs to no player. It must still hear
// why, and must not linger in the room where later broadcasts would try to
// build a view for a player that does not exist.
func TestRejectedJoinDoesNotBreakRoom(t *testing.T) {
	base := startServer(t)
	alice, bob, start := startTwoPlayerGame(t, base, "REJ001")

	late := dial(t, base, "REJ001", "Late", nil)
	if e := decode[protocol.ErrorData](t, late.await("error")); e.Code != "GAME_IN_PROGRESS" {
		t.Fatalf("late joiner: want GAME_IN_PROGRESS, got %q", e.Code)
	}

	mover := alice
	if start.CurrentTurn == bob.PlayerID {
		mover = bob
	}
	moveN(mover.testClient)
	if v := decode[view](t, alice.await("turn_result")); v.Turn <= start.Turn {
		t.Fatalf("turn did not advance after the rejected join: %d -> %d", start.Turn, v.Turn)
	}
}

// A player whose connection dropped rejoins with the credentials from their
// welcome and must be able to keep playing.
func TestReconnectedPlayerCanAct(t *testing.T) {
	base := startServer(t)
	alice, bob, start := startTwoPlayerGame(t, base, "REC001")
	mover := alice
	if start.CurrentTurn == bob.PlayerID {
		mover = bob
	}

	mover.ws.CloseNow()
	time.Sleep(100 * time.Millisecond)
	back := dial(t, base, "REC001", mover.name, reconnectQuery(mover.PlayerID, mover.ReconnectToken))
	if w := decode[welcome](t, back.await("welcome")); w.PlayerID != mover.PlayerID {
		t.Fatalf("reconnect welcome: want playerId %s, got %s", mover.PlayerID, w.PlayerID)
	}
	back.await("turn_result") // state resync

	moveN(back)
	res, ok := back.next("turn_result", awaitTimeout)
	if !ok {
		t.Fatal("reconnected player's move got no turn_result — the move was ignored")
	}
	if v := decode[view](t, res); v.Turn <= start.Turn {
		t.Fatalf("turn did not advance: %d -> %d", start.Turn, v.Turn)
	}
}

// Mobile clients usually reconnect before the server notices the old socket
// is dead. When that old socket finally goes away it must not take the
// player's new connection with it.
func TestStaleSocketClosingKeepsReconnectedPlayer(t *testing.T) {
	base := startServer(t)
	alice, bob, start := startTwoPlayerGame(t, base, "REC002")
	mover := alice
	if start.CurrentTurn == bob.PlayerID {
		mover = bob
	}

	back := dial(t, base, "REC002", mover.name, reconnectQuery(mover.PlayerID, mover.ReconnectToken))
	back.await("welcome")
	back.await("turn_result")
	mover.ws.CloseNow() // the stale socket finally dies
	time.Sleep(100 * time.Millisecond)

	moveN(back)
	if _, ok := back.next("turn_result", awaitTimeout); !ok {
		t.Fatal("reconnected socket stopped working once the stale socket closed")
	}
}

// Player ids are public — every view lists them — so an id alone, or an id
// paired with someone else's token, must never resume another player's
// session.
func TestReconnectNeedsThatPlayersToken(t *testing.T) {
	cases := []struct {
		name  string
		token func(alice, bob *player) string
	}{
		{"no token", func(_, _ *player) string { return "" }},
		{"wrong token", func(_, _ *player) string { return "not-a-real-token" }},
		{"another player's token", func(_, bob *player) string { return bob.ReconnectToken }},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := startServer(t)
			room := "SPY00" + string(rune('1'+i))
			alice, bob, _ := startTwoPlayerGame(t, base, room)

			// Not a valid reconnect, so it is a fresh join into a running game.
			spy := dial(t, base, room, "Bob", reconnectQuery(alice.PlayerID, tc.token(alice, bob)))
			if env := spy.first(); env.Type != "error" ||
				decode[protocol.ErrorData](t, env.Data).Code != "GAME_IN_PROGRESS" {
				t.Fatalf("spy: want GAME_IN_PROGRESS error, got %s %s", env.Type, env.Data)
			}

			// Alice's own socket is untouched and still receives the game.
			alice.send("action", protocol.ActionData{Kind: protocol.ActionPause})
			alice.await("turn_result")
		})
	}
}
