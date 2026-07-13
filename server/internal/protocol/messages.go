package protocol

import (
	"encoding/json"

	"github.com/your-org/blindmap/internal/game"
)

type Envelope struct {
	Type string          `json:"type"`
	Ts   int64           `json:"ts"`
	Data json.RawMessage `json:"data"`
}

type JoinData struct {
	RoomCode      string `json:"roomCode"`
	PlayerName    string `json:"playerName"`
	ClientVersion string `json:"clientVersion"`
	PlayerID      string `json:"playerId,omitempty"`
}

type ActionKind string

const (
	ActionMove      ActionKind = "move"
	ActionShoot     ActionKind = "shoot"
	ActionSubmitMap ActionKind = "submit_map"
	ActionPause     ActionKind = "pause"
	ActionResume    ActionKind = "resume"
	ActionPass      ActionKind = "pass" // bot/test: skip turn without waiting for timer
	ActionAddBot    ActionKind = "add_bot"    // lobby, host only
	ActionRemoveBot ActionKind = "remove_bot" // lobby, host only
)

type ActionData struct {
	Kind            ActionKind      `json:"kind"`
	Direction       string          `json:"direction,omitempty"`
	ItemID          string          `json:"itemId,omitempty"`
	Walls           []game.Position `json:"walls,omitempty"`
	MapSize         int             `json:"mapSize,omitempty"`
	TurnSeconds     int             `json:"turnSeconds,omitempty"`
	DebugMap        bool            `json:"debugMap,omitempty"`
	ControlMap      bool            `json:"controlMap,omitempty"`
	ControlScenario int             `json:"controlScenario,omitempty"`
	Difficulty      string          `json:"difficulty,omitempty"` // add_bot: easy|medium|hard
	BotID           string          `json:"botId,omitempty"`      // remove_bot: id or name; empty = last added
}

type EventKind string

const (
	EventPlayerMoved      EventKind = "player_moved"
	EventTrapTriggered    EventKind = "trap_triggered"
	EventRewardActivated  EventKind = "reward_activated"
	EventPlayerEliminated EventKind = "player_eliminated"
	EventYouEliminated    EventKind = "you_were_eliminated"
	EventShotFired        EventKind = "shot_fired"
	EventMapSubmitted     EventKind = "map_submitted"
	EventPortalUsed       EventKind = "portal_used"
	EventTurnSkipped      EventKind = "turn_skipped"
	EventInfoRevealed     EventKind = "info_revealed"
)

type Event struct {
	Kind        EventKind     `json:"kind"`
	Payload     interface{}   `json:"payload,omitempty"`
	ForPlayerID game.PlayerID `json:"-"` // server-side filter only; not sent on wire
}

type WelcomeData struct {
	PlayerID  string      `json:"playerId"`
	RoomState interface{} `json:"roomState"`
}

type LobbyView struct {
	RoomCode string   `json:"roomCode"`
	Players  []string `json:"players"`
	IsHost   bool     `json:"isHost"`
}

type GameOverData struct {
	Winner    *string `json:"winner"`
	WinReason string  `json:"winReason"`
}

type ErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ServerShutdownData struct {
	ReconnectAfterMs int `json:"reconnectAfterMs"`
}

// ChatData is sent client→server when a player sends a chat message.
type ChatData struct {
	Text string `json:"text"`
}

// ChatMsgData is broadcast server→all clients in the room.
type ChatMsgData struct {
	SenderName string `json:"senderName"`
	Ts         int64  `json:"ts"`
	Text       string `json:"text"`
}

// ChatHistoryData is sent to a newly joined client with the room's recent history.
type ChatHistoryData struct {
	Messages []ChatMsgData `json:"messages"`
}
