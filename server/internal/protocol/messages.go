package protocol

import "encoding/json"

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
	ActionPickup    ActionKind = "pickup"
	ActionShoot     ActionKind = "shoot"
	ActionSubmitMap ActionKind = "submit_map"
)

type ActionData struct {
	Kind      ActionKind `json:"kind"`
	Direction string     `json:"direction,omitempty"`
	ItemID    string     `json:"itemId,omitempty"`
}

type EventKind string

const (
	EventTrapTriggered    EventKind = "trap_triggered"
	EventRewardActivated  EventKind = "reward_activated"
	EventPlayerEliminated EventKind = "player_eliminated"
	EventShotFired        EventKind = "shot_fired"
	EventMapSubmitted     EventKind = "map_submitted"
	EventPortalUsed       EventKind = "portal_used"
	EventClueReceived     EventKind = "clue_received"
	EventTurnSkipped      EventKind = "turn_skipped"
)

type Event struct {
	Kind    EventKind   `json:"kind"`
	Payload interface{} `json:"payload,omitempty"`
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
