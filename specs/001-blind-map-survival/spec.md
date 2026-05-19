# Feature Specification: Blind Map Survival

**Feature Branch**: `001-blind-map-survival`
**Created**: 2026-05-16
**Status**: Draft
**Input**: User description: "Blind Map Survival — multiplayer turn-based survival & deduction game"

## Clarifications

### Session 2026-05-16

- Q: When a bullet is fired, does it eliminate the first player hit instantly or apply damage? → A: Instant elimination — bullet stops at first player in its path and removes that player from the game immediately.
- Q: Is the map-completion win triggered by an explicit player action or automatically by the server? → A: Explicit "Submit Map" action — the player chooses it as their turn action; the server then validates and awards the win.
- Q: How is a tie resolved if two players complete the map on the same turn? → A: Not possible — turns are strictly sequential, so only one player can submit per turn; no tie-break rule needed.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Create and Join a Private Room (Priority: P1)

A player wants to start a game session and invite friends. They create a private room and share a short code. Friends enter the code to join before the game begins.

**Why this priority**: Room management is the entry point to all gameplay — nothing else works without it.

**Independent Test**: Can be fully tested by one player creating a room, sharing the 6-character code, and 1–7 other players joining successfully, then the host starting the game.

**Acceptance Scenarios**:

1. **Given** a player opens the app, **When** they choose "Create Room", **Then** a unique 6-character alphanumeric room code is displayed and the player is in the lobby as host.
2. **Given** a room code is shared, **When** another player enters it and joins, **Then** both players appear in the lobby and see each other's names.
3. **Given** 2–8 players are in the lobby, **When** the host starts the game, **Then** all players transition to the active game screen simultaneously.
4. **Given** 9 or more players attempt to join the same room, **When** the 9th player tries to join, **Then** they receive a "Room Full" error.

---

### User Story 2 - Take Turns and Navigate the Map (Priority: P1)

During an active game, each player sees their own position on the map and takes one action per turn — moving in a cardinal direction or picking up an item — within a 30-second window.

**Why this priority**: The core turn-based movement loop is the foundation of all gameplay interaction.

**Independent Test**: Can be tested with 2 players taking alternating turns, each moving in any direction, with state updates visible after each turn resolves.

**Acceptance Scenarios**:

1. **Given** it is Player A's turn, **When** Player A moves North, **Then** their position updates and Player B's turn begins.
2. **Given** a player's 30-second turn timer expires, **When** no action is submitted, **Then** the turn ends automatically and the next player's turn begins.
3. **Given** two players are on the same cell, **When** either moves, **Then** no collision event occurs and neither player receives notification of the shared cell.
4. **Given** a player reaches the map boundary, **When** they attempt to move outside it, **Then** the move is rejected and they may try another direction.

---

### User Story 3 - Collect Items and Use Clues to Deduce Opponent Locations (Priority: P2)

Players find items scattered on the map that provide indirect information about opponents or confer advantages. Strategic use of these clues is the core deduction mechanic.

**Why this priority**: The deduction mechanic differentiates this game; without items the game is only movement.

**Independent Test**: Can be tested by placing a compass item on the map, having a player pick it up, and verifying the correct directional or positional clue is returned to only that player.

**Acceptance Scenarios**:

1. **Given** a player steps on a bullet tile, **When** they pick it up, **Then** they acquire a bullet and the tile remains available for future pickups.
2. **Given** a player picks up a compass item, **When** the item is used, **Then** they receive exactly one of the four defined compass clues (nearest player direction / own starting position / one other player's current position / 3×3 surroundings of self) and no other player receives that information.
3. **Given** a player steps on a reward tile, **When** the reward triggers, **Then** one of the three reward effects is applied (all positions revealed to all / nearest player direction to this player / all bullet tile locations to this player) and the tile repositions randomly after activation.
4. **Given** a player steps on a trap tile, **When** the trap triggers, **Then** exactly one random penalty applies (position revealed to all / random teleport / lose next turn / lose held bullet / information blackout for next turn).
5. **Given** a teleport portal pair exists, **When** a player steps into one portal, **Then** they immediately appear at the paired portal's location.

---

### User Story 4 - Eliminate Opponents via Shooting (Priority: P2)

A player who has collected a bullet may shoot in a cardinal direction during their turn, potentially eliminating an opponent.

**Why this priority**: Combat is the primary elimination mechanic for the "last alive" win condition.

**Independent Test**: Can be tested by giving one player a bullet and another player a known position in the shooting path, verifying the targeted player is eliminated.

**Acceptance Scenarios**:

1. **Given** a player holds a bullet, **When** they choose to shoot in a direction, **Then** the bullet is consumed, travels in that direction, stops at the first player it hits (eliminating them instantly), or travels to the map boundary if no player is in its path.
2. **Given** a player is eliminated, **When** they are removed from the game, **Then** their name is shown as dead in the player list and they can observe the remainder of the game.
3. **Given** a player shoots in a direction with no other player in that line, **When** the shot resolves, **Then** the bullet is consumed and no elimination occurs.

---

### User Story 5 - Win by Mapping the Entire Board (Priority: P3)

As an alternative to combat, a player may win by being the first to fully map the game board and successfully submit it.

**Why this priority**: Provides a non-combat win path that rewards exploration over aggression.

**Independent Test**: Can be tested by having one player visit every cell on a small map and submit, verifying they are declared winner.

**Acceptance Scenarios**:

1. **Given** a player has visited every cell on the map, **When** they choose "Submit Map" as their turn action, **Then** the server validates the submission and declares them the winner, ending the game for all players.
2. **Given** a player has not visited every cell, **When** they attempt to submit, **Then** the submission is rejected with an indication of how many cells remain unvisited.
3. **Given** turns are strictly sequential, **When** only one player can act per turn, **Then** simultaneous map submission is not possible; the first player to submit a valid complete map on their turn wins.

---

### User Story 6 - Reconnect After Disconnection (Priority: P3)

If a player's connection drops, they can reconnect to their ongoing session and resume play without losing their game slot.

**Why this priority**: Network interruptions are common, especially on mobile; losing a game slot discourages play.

**Independent Test**: Can be tested by disconnecting mid-game, reconnecting within 5 attempts, and verifying the player's state (position, items, health) is fully restored.

**Acceptance Scenarios**:

1. **Given** a player's connection drops during an active game, **When** they reconnect within the retry window, **Then** their player slot is preserved and they rejoin to the current game state.
2. **Given** a player is disconnected during their turn, **When** their 30-second timer expires, **Then** their turn is skipped automatically and the next player's turn begins.
3. **Given** a player fails to reconnect after 5 attempts, **When** reconnection is exhausted, **Then** they are shown a connection failure screen.

---

### Edge Cases

- What happens if the host disconnects before starting the game? (Another player should be promoted to host or the room should remain open for the host to rejoin.)
- What happens if only 1 player remains because all others disconnected? (The remaining player should be declared winner.)
- What happens if a random teleport lands a player on a trap tile? (The trap on the destination tile should not re-trigger immediately.)
- What happens if a reward tile reposition places it on an occupied or special tile?
- What if the map has no bullet tiles reachable before all players are adjacent?

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST support 2–8 players per game room.
- **FR-002**: System MUST generate a unique 6-character alphanumeric room code for each created room.
- **FR-003**: Players MUST be able to join a room by entering the room code without creating an account.
- **FR-004**: The host player MUST be able to start the game when 2 or more players are in the lobby.
- **FR-005**: System MUST enforce a 30-second turn timer; if a player does not act, their turn is skipped automatically.
- **FR-006**: Turn order MUST be sequential (Player A → B → C → … → last player → back to A).
- **FR-007**: System MUST hide all players' positions from each other; no player may see another player's location directly.
- **FR-008**: Players MUST be able to move one cell per turn in any cardinal direction (N/S/E/W) that is within map bounds.
- **FR-009**: Players MUST be able to pick up an item on their current cell as their turn action.
- **FR-010**: Bullet tiles MUST allow unlimited pickups; the tile does not disappear after pickup.
- **FR-011**: Reward tiles MUST apply one of three random effects on activation and reposition randomly afterward.
- **FR-012**: Compass items MUST reveal one of four defined clues to only the player who uses it.
- **FR-013**: Trap tiles MUST apply one of five random penalties to the player who steps on them.
- **FR-014**: Teleport portals MUST come in connected pairs (always an even count); entering one exits the paired portal.
- **FR-015**: System MUST NOT reveal any player's starting position at game start.
- **FR-016**: Players sharing a cell MUST NOT be notified of the shared occupancy.
- **FR-017**: A player with a bullet MUST be able to shoot in a cardinal direction as their turn action, consuming the bullet.
- **FR-018**: System MUST instantly eliminate any player hit by a bullet; a shot travels in a straight line and stops at the first player in its path.
- **FR-019**: The game MUST end when only one player remains alive, declaring that player the winner.
- **FR-020**: Players MUST be able to choose "Submit Map" as a turn action; the server validates that the submitting player has visited every cell, and if valid, ends the game and declares that player the winner.
- **FR-021**: System MUST support reconnection: a disconnected player's slot is preserved; they may rejoin using their original identity.
- **FR-022**: System MUST be playable via a terminal command-line interface on Linux and Windows.
- **FR-023**: System MUST be playable via an Android mobile app.
- **FR-024**: All game state decisions MUST be made by the server; the client is a display-only interface.

### Key Entities *(include if feature involves data)*

- **Room**: A game session identified by a 6-character code. Has a phase (Lobby / Active / Ended), player list, map, and turn state. Destroyed after game ends (no persistence).
- **Player**: Identified by name (no account). Has a position (hidden from others), health/alive status, and inventory. Starting position is unknown to all players.
- **Map**: A grid of cells (size TBD at planning). Contains placed items, hazards, and terrain. Each player tracks which cells they have personally visited.
- **Cell**: One grid square. May contain: bullet tile, reward tile, trap tile, teleport portal, or be empty.
- **Item**: Held in a player's inventory after pickup. Currently defined: Bullet, Compass.
- **Trap tile**: A pre-placed hazard that triggers a random penalty when a player steps on it.
- **Teleport Portal**: A paired cell that transports the player to its partner cell on entry.
- **Turn**: One player's action window (30 seconds). One action per turn: Move, Pickup, or Shoot.
- **Event**: A discrete game notification (clue revealed, trap triggered, player eliminated, turn result). Each event is scoped to the recipient player(s) only.

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A complete game session (lobby → gameplay → winner declared) can be completed end-to-end by 2 players in under 20 minutes.
- **SC-002**: Players experience no perceptible delay between submitting a turn action and receiving the updated game state (state update delivered within 3 seconds of action submission under normal network conditions).
- **SC-003**: A disconnected player can successfully rejoin an ongoing game within 60 seconds of disconnection without losing their game state.
- **SC-004**: The server correctly withholds all opponent position information in 100% of game states — verified by inspection of data sent to each client.
- **SC-005**: The cold-start connection flow (server waking from idle) completes within 60 seconds, with visible progress feedback to the player.
- **SC-006**: The game supports a full 8-player match on free-tier cloud infrastructure without crashing or losing game state mid-match.
- **SC-007**: 90% of playtests with first-time players result in the player correctly understanding their turn options without external guidance.

---

## Assumptions

- Map size will be determined during planning (likely 15×15 to 25×25 for 2–8 players); it is a configurable server-side parameter.
- A player is "eliminated" when shot. The health system (instant elimination vs. multi-hit) is marked for clarification (FR-C1).
- The map submission win condition requires a player to have personally visited every cell; the server tracks visited cells per player.
- Trap tiles are pre-placed by the server during map generation; players cannot place traps in Phase 1 (Phase 2 introduces a placeable trap item).
- Signal Flare is a Phase 2 item; it is out of scope for the MVP spec.
- Spectator mode (watching an ongoing game) is a Phase 2 feature; eliminated players may only observe, not interact.
- Desktop TUI client is a Phase 2 deliverable; Phase 1 focuses on Android client only.
- No persistent data (scores, history, accounts) is stored; each game is self-contained.
- Players who disconnect and do not reconnect before their turn timer expires have their turn skipped; they are not auto-eliminated.
- The reward tile repositions to a random empty, non-special cell after activation.
- Teleport portal pairs are fixed at map generation and do not move during the game.
