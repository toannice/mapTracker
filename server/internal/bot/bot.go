// Package bot implements fair-mode computer players. A Bot only ever consumes
// protocol.PlayerView — the exact same filtered view a human client receives on
// the wire — so it cannot see hidden map cells or other players' positions
// beyond what public events reveal. Fairness is guaranteed by construction.
package bot

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sort"

	"github.com/your-org/blindmap/internal/game"
	"github.com/your-org/blindmap/internal/protocol"
)

type Difficulty string

const (
	Easy   Difficulty = "easy"   // random walk, bumps into walls
	Medium Difficulty = "medium" // frontier exploration, avoids known traps, ε=0.15 mistakes
	Hard   Difficulty = "hard"   // exploration + opponent tracking from public events + shoot + map submit
)

// ParseDifficulty returns the difficulty for s, defaulting to Medium when s is
// empty and reporting ok=false for unknown values.
func ParseDifficulty(s string) (Difficulty, bool) {
	switch Difficulty(s) {
	case Easy, Medium, Hard:
		return Difficulty(s), true
	case "":
		return Medium, true
	}
	return "", false
}

const mediumEpsilon = 0.15 // chance a medium bot makes a random move instead of the planned one

type Bot struct {
	ID         game.PlayerID
	Name       string
	Difficulty Difficulty

	mapSize    int
	selfPos    game.Position
	knownWalls map[game.Position]bool
	knownFloor map[game.Position]bool          // cells known NOT to be walls
	knownKind  map[game.Position]game.CellKind // last known kind (visited or revealed)
	// candidates tracks where each living opponent might be, keyed by player
	// name (event payloads identify players by name, not id).
	candidates map[string]map[game.Position]bool
	dead       map[string]bool

	pendingMoveTarget *game.Position // set when we attempt a move; resolved by our next player_moved event
	submitFailed      bool           // a "certain" submit missed → logic gap; never try again
}

func New(id game.PlayerID, name string, diff Difficulty) *Bot {
	return &Bot{
		ID:         id,
		Name:       name,
		Difficulty: diff,
		knownWalls: make(map[game.Position]bool),
		knownFloor: make(map[game.Position]bool),
		knownKind:  make(map[game.Position]game.CellKind),
		candidates: make(map[string]map[game.Position]bool),
		dead:       make(map[string]bool),
	}
}

// Observe digests a PlayerView broadcast (turn_result / game_start) into the
// bot's memory. Must be called from the room goroutine.
func (b *Bot) Observe(view *protocol.PlayerView) {
	if view.MapStats != nil {
		b.mapSize = view.MapStats.MapSize
	}
	b.selfPos = view.Self.Pos
	for _, cv := range view.VisibleMap {
		b.knownFloor[cv.Pos] = true
		b.knownKind[cv.Pos] = cv.Kind
	}

	lastMoved := "" // name from the batch's player_moved, for portal/trap attribution
	for _, ev := range view.Events {
		switch ev.Kind {
		case protocol.EventPlayerMoved:
			var p struct {
				PlayerName string `json:"playerName"`
				Direction  string `json:"direction"`
				Success    bool   `json:"success"`
				BlockType  string `json:"blockType"`
			}
			if !decode(ev.Payload, &p) {
				continue
			}
			lastMoved = p.PlayerName
			if p.PlayerName == b.Name {
				if !p.Success && b.pendingMoveTarget != nil && b.inBounds(*b.pendingMoveTarget) {
					b.knownWalls[*b.pendingMoveTarget] = true
				}
				b.pendingMoveTarget = nil
			} else {
				b.observeOpponentMove(p.PlayerName, p.Direction, p.Success, p.BlockType)
			}

		case protocol.EventPortalUsed:
			// Destination is hidden from everyone — an opponent who teleported
			// could be anywhere again.
			if lastMoved != "" && lastMoved != b.Name {
				b.candidates[lastMoved] = b.fullCandidateSet()
			}

		case protocol.EventTrapTriggered:
			var p struct {
				Effect     string         `json:"effect"`
				PlayerName string         `json:"playerName"`
				Pos        *game.Position `json:"pos"`
			}
			if !decode(ev.Payload, &p) {
				continue
			}
			switch p.Effect {
			case "reveal_position":
				if p.PlayerName != "" && p.PlayerName != b.Name && p.Pos != nil {
					b.setCandidate(p.PlayerName, *p.Pos)
				}
			case "random_teleport":
				if lastMoved != "" && lastMoved != b.Name {
					b.candidates[lastMoved] = b.fullCandidateSet()
				}
			}

		case protocol.EventRewardActivated:
			var p struct {
				Effect    string `json:"effect"`
				Positions []struct {
					Name string        `json:"name"`
					Pos  game.Position `json:"pos"`
				} `json:"positions"`
				Locations []game.Position `json:"locations"`
			}
			if !decode(ev.Payload, &p) {
				continue
			}
			switch p.Effect {
			case "all_positions_revealed":
				for _, np := range p.Positions {
					if np.Name != b.Name {
						b.setCandidate(np.Name, np.Pos)
					}
				}
			case "all_bullet_locations":
				for _, loc := range p.Locations {
					b.knownFloor[loc] = true
					b.knownKind[loc] = game.CellBullet
				}
			}

		case protocol.EventInfoRevealed: // private; BuildPlayerView already filtered to ours
			var p struct {
				Type  string `json:"type"`
				Cells []struct {
					Rel  string        `json:"rel"`
					Kind game.CellKind `json:"kind"`
				} `json:"cells"`
				PlayerName string         `json:"playerName"`
				Pos        *game.Position `json:"pos"`
			}
			if !decode(ev.Payload, &p) {
				continue
			}
			switch p.Type {
			case "surroundings_3x3":
				// rel is "row-col" with 0..2 range, center 1-1 = our cell. The
				// payload reports out-of-bounds cells as walls, so re-check bounds.
				for _, rc := range p.Cells {
					var row, col int
					if _, err := fmt.Sscanf(rc.Rel, "%d-%d", &row, &col); err != nil {
						continue
					}
					abs := game.Position{X: b.selfPos.X + col - 1, Y: b.selfPos.Y + row - 1}
					if !b.inBounds(abs) {
						continue
					}
					if rc.Kind == game.CellWall {
						b.knownWalls[abs] = true
					} else {
						b.knownFloor[abs] = true
						b.knownKind[abs] = rc.Kind
					}
				}
			case "player_position":
				if p.PlayerName != "" && p.PlayerName != b.Name && p.Pos != nil {
					b.setCandidate(p.PlayerName, *p.Pos)
				}
			}

		case protocol.EventPlayerEliminated:
			var p struct {
				PlayerName string `json:"playerName"`
			}
			if decode(ev.Payload, &p) {
				b.dead[p.PlayerName] = true
				delete(b.candidates, p.PlayerName)
			}

		case protocol.EventMapSubmitted:
			var p struct {
				PlayerName string `json:"playerName"`
				Correct    bool   `json:"correct"`
			}
			if decode(ev.Payload, &p) && p.PlayerName == b.Name && !p.Correct {
				b.submitFailed = true
			}
		}
	}
}

// Decide picks this turn's actions: at most one submit_map (free) followed by
// exactly one turn-consuming action. view carries no events — Observe already
// consumed them from broadcasts.
func (b *Bot) Decide(view *protocol.PlayerView, rng *rand.Rand) []protocol.ActionData {
	b.Observe(view) // refresh selfPos / visible cells

	var out []protocol.ActionData
	if b.Difficulty == Hard {
		if a, ok := b.maybeSubmit(view); ok {
			out = append(out, a)
		}
		if a, ok := b.maybeShoot(view); ok {
			return append(out, a)
		}
	}

	wantBullet := b.Difficulty == Hard && !hasBullet(view.Self.Inventory)
	dir := b.chooseMove(rng, wantBullet)
	if dir == "" {
		return append(out, protocol.ActionData{Kind: protocol.ActionPass})
	}
	dx, dy := compassDelta(dir)
	t := game.Position{X: b.selfPos.X + dx, Y: b.selfPos.Y + dy}
	b.pendingMoveTarget = &t
	return append(out, protocol.ActionData{Kind: protocol.ActionMove, Direction: dir})
}

// maybeSubmit fires only when the bot's known wall set provably equals the real
// one (count from public MapStats) — a guaranteed win.
func (b *Bot) maybeSubmit(view *protocol.PlayerView) (protocol.ActionData, bool) {
	if b.submitFailed || view.Self.SubmitsLeft <= 0 || view.MapStats == nil {
		return protocol.ActionData{}, false
	}
	if len(b.knownWalls) != view.MapStats.Counts["wall"] {
		return protocol.ActionData{}, false
	}
	walls := make([]game.Position, 0, len(b.knownWalls))
	for w := range b.knownWalls {
		walls = append(walls, w)
	}
	sort.Slice(walls, func(i, j int) bool {
		if walls[i].Y != walls[j].Y {
			return walls[i].Y < walls[j].Y
		}
		return walls[i].X < walls[j].X
	})
	return protocol.ActionData{Kind: protocol.ActionSubmitMap, Walls: walls}, true
}

// maybeShoot fires when an opponent's candidate set is small and concentrated
// along one shooting ray from our position.
func (b *Bot) maybeShoot(view *protocol.PlayerView) (protocol.ActionData, bool) {
	if !hasBullet(view.Self.Inventory) {
		return protocol.ActionData{}, false
	}
	bestRatio := 0.0
	bestDir := ""
	for _, other := range view.Others {
		if !other.Alive || b.dead[other.Name] {
			continue
		}
		set := b.candidates[other.Name]
		if len(set) == 0 || len(set) > 6 {
			continue
		}
		for _, dir := range compassDirs {
			hits := 0
			for _, cell := range b.ray(dir) {
				if set[cell] {
					hits++
				}
			}
			ratio := float64(hits) / float64(len(set))
			if ratio >= 0.6 && ratio > bestRatio {
				bestRatio = ratio
				bestDir = dir
			}
		}
	}
	if bestDir == "" {
		return protocol.ActionData{}, false
	}
	return protocol.ActionData{Kind: protocol.ActionShoot, Direction: bestDir}, true
}

// ray lists the cells a bullet would traverse from selfPos in dir: it stops at
// the border and at known walls; unknown cells are optimistically passable.
func (b *Bot) ray(dir string) []game.Position {
	dx, dy := compassDelta(dir)
	var cells []game.Position
	pos := b.selfPos
	for {
		pos = game.Position{X: pos.X + dx, Y: pos.Y + dy}
		if !b.inBounds(pos) || b.knownWalls[pos] {
			return cells
		}
		cells = append(cells, pos)
	}
}

func (b *Bot) chooseMove(rng *rand.Rand, wantBullet bool) string {
	switch b.Difficulty {
	case Easy:
		return compassDirs[rng.IntN(len(compassDirs))]
	case Medium:
		if rng.Float64() < mediumEpsilon {
			if dir := b.randomSafeDir(rng); dir != "" {
				return dir
			}
		}
	}
	if dir := b.exploreDirection(rng, true, wantBullet); dir != "" {
		return dir
	}
	// No trap-free route — allow crossing known traps.
	if dir := b.exploreDirection(rng, false, wantBullet); dir != "" {
		return dir
	}
	return b.randomSafeDir(rng)
}

// exploreDirection BFS-plans toward the most useful frontier and returns the
// first step. Unknown cells are treated as passable (bumping reveals walls).
// When wantBullet is set (hard bot, empty-handed) known bullet tiles take
// priority over frontier cells.
func (b *Bot) exploreDirection(rng *rand.Rand, avoidTraps bool, wantBullet bool) string {
	if b.mapSize == 0 {
		return ""
	}
	goal := func(pos game.Position) bool {
		return !b.knownFloor[pos] && !b.knownWalls[pos] // truly unknown cell
	}
	if wantBullet {
		var bullets []game.Position
		for pos, kind := range b.knownKind {
			if kind == game.CellBullet && pos != b.selfPos {
				bullets = append(bullets, pos)
			}
		}
		if len(bullets) > 0 {
			bulletSet := make(map[game.Position]bool, len(bullets))
			for _, p := range bullets {
				bulletSet[p] = true
			}
			goal = func(pos game.Position) bool { return bulletSet[pos] }
		}
	}

	blocked := func(pos game.Position) bool {
		if b.knownWalls[pos] {
			return true
		}
		return avoidTraps && b.knownKind[pos] == game.CellTrap
	}

	// Randomize direction preference so identical bots don't move in lockstep.
	dirs := append([]string(nil), compassDirs...)
	rng.Shuffle(len(dirs), func(i, j int) { dirs[i], dirs[j] = dirs[j], dirs[i] })

	type node struct {
		pos      game.Position
		firstDir string
	}
	visited := map[game.Position]bool{b.selfPos: true}
	queue := []node{{pos: b.selfPos}}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, dir := range dirs {
			dx, dy := compassDelta(dir)
			next := game.Position{X: n.pos.X + dx, Y: n.pos.Y + dy}
			if !b.inBounds(next) || visited[next] || blocked(next) {
				continue
			}
			first := n.firstDir
			if first == "" {
				first = dir
			}
			if goal(next) {
				return first
			}
			visited[next] = true
			queue = append(queue, node{pos: next, firstDir: first})
		}
	}
	return ""
}

// randomSafeDir picks a random direction whose target is in bounds and not a
// known wall. Returns "" when boxed in.
func (b *Bot) randomSafeDir(rng *rand.Rand) string {
	var options []string
	for _, dir := range compassDirs {
		dx, dy := compassDelta(dir)
		t := game.Position{X: b.selfPos.X + dx, Y: b.selfPos.Y + dy}
		if b.inBounds(t) && !b.knownWalls[t] {
			options = append(options, dir)
		}
	}
	if len(options) == 0 {
		return ""
	}
	return options[rng.IntN(len(options))]
}

// observeOpponentMove narrows an opponent's candidate set from a public
// player_moved event (direction + success + landed block type).
func (b *Bot) observeOpponentMove(name, friendlyDir string, success bool, blockType string) {
	dx, dy, ok := friendlyDelta(friendlyDir)
	if !ok || b.mapSize == 0 {
		return
	}
	set := b.candidateSet(name)
	next := make(map[game.Position]bool, len(set))
	for c := range set {
		t := game.Position{X: c.X + dx, Y: c.Y + dy}
		if success {
			if b.inBounds(t) && !b.knownWalls[t] && b.kindCompatible(t, blockType) {
				next[t] = true
			}
		} else {
			// Move failed: target must be border, wall, or a cell we can't rule out.
			if !b.inBounds(t) || b.knownWalls[t] || !b.knownFloor[t] {
				next[c] = true
			}
		}
	}
	if len(next) == 0 {
		next = b.fullCandidateSet() // lost track — reset
	}
	b.candidates[name] = next
}

// kindCompatible reports whether a cell we may know about could have produced
// the announced blockType. Reward tiles relocate after activation, so any
// comparison involving "reward" stays compatible.
func (b *Bot) kindCompatible(pos game.Position, blockType string) bool {
	kind, known := b.knownKind[pos]
	if !known || blockType == "" {
		return true
	}
	expected := blockOf(kind)
	if expected == blockType {
		return true
	}
	return expected == "reward" || blockType == "reward"
}

func blockOf(kind game.CellKind) string {
	switch kind {
	case game.CellBullet:
		return "bullet"
	case game.CellReward:
		return "reward"
	case game.CellTrap:
		return "trap"
	case game.CellPortalA, game.CellPortalB:
		return "portal"
	case game.CellInfo:
		return "info"
	default:
		return "blank"
	}
}

func (b *Bot) setCandidate(name string, pos game.Position) {
	b.knownFloor[pos] = true // a player stands there → not a wall
	b.candidates[name] = map[game.Position]bool{pos: true}
}

func (b *Bot) candidateSet(name string) map[game.Position]bool {
	if s, ok := b.candidates[name]; ok && len(s) > 0 {
		return s
	}
	s := b.fullCandidateSet()
	b.candidates[name] = s
	return s
}

func (b *Bot) fullCandidateSet() map[game.Position]bool {
	s := make(map[game.Position]bool, b.mapSize*b.mapSize)
	for y := 0; y < b.mapSize; y++ {
		for x := 0; x < b.mapSize; x++ {
			pos := game.Position{X: x, Y: y}
			if !b.knownWalls[pos] {
				s[pos] = true
			}
		}
	}
	return s
}

func (b *Bot) inBounds(pos game.Position) bool {
	return pos.X >= 0 && pos.X < b.mapSize && pos.Y >= 0 && pos.Y < b.mapSize
}

func hasBullet(inv []game.Item) bool {
	for _, item := range inv {
		if item.Kind == game.ItemBullet {
			return true
		}
	}
	return false
}

var compassDirs = []string{"N", "S", "E", "W"}

func compassDelta(dir string) (int, int) {
	switch dir {
	case "N":
		return 0, -1
	case "S":
		return 0, 1
	case "E":
		return 1, 0
	case "W":
		return -1, 0
	}
	return 0, 0
}

func friendlyDelta(dir string) (int, int, bool) {
	switch dir {
	case "up":
		return 0, -1, true
	case "down":
		return 0, 1, true
	case "right":
		return 1, 0, true
	case "left":
		return -1, 0, true
	}
	return 0, 0, false
}

// decode round-trips an event payload (arbitrary map/struct) into a typed
// struct — the same shape a client would parse off the wire.
func decode(payload interface{}, out interface{}) bool {
	raw, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}
