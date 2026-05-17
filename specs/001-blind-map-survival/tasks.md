# Tasks: Blind Map Survival

**Input**: Design documents from `specs/001-blind-map-survival/`
**Prerequisites**: plan.md âœ“, spec.md âœ“, research.md âœ“, data-model.md âœ“, contracts/websocket-protocol.md âœ“, quickstart.md âœ“

**Organization**: Tasks grouped by user story â€” each story independently implementable and testable.
**Phase 1 MVP scope**: US1 (Room join) + US2 (Turn navigation) â†’ deployable to Render.com.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on prior incomplete tasks)
- **[Story]**: Which user story (US1â€“US6 from spec.md)
- Each task includes exact file path(s)

---

## Phase 1: Setup

**Purpose**: Create the full project skeleton â€” directories, module files, build configs â€” so all subsequent phases have a concrete target to write into.

- [x] T001 Create server directory tree: `server/cmd/server/`, `server/internal/config/`, `server/internal/hub/`, `server/internal/room/`, `server/internal/game/`, `server/internal/conn/`, `server/internal/protocol/`
- [x] T002 [P] Initialize Go module: create `server/go.mod` (module `github.com/your-org/blindmap`, go 1.23); run `go get github.com/coder/websocket@v1.8` inside `server/`; commit resulting `server/go.sum`
- [x] T003 [P] Create `client/settings.gradle.kts` declaring subprojects `shared` and `androidApp`; create `client/build.gradle.kts` (root Gradle with `plugins { kotlin("multiplatform") apply false; id("com.android.application") apply false }`)
- [x] T004 [P] Create `client/shared/build.gradle.kts`: apply `kotlin("multiplatform")`, declare `androidTarget()` + `jvm()` targets, add `kotlinx.serialization` (1.7+), Ktor Client (`ktor-client-core`, `ktor-client-websockets`, `ktor-client-okhttp` for androidMain, `ktor-client-cio` for jvmMain), Kotlin Coroutines (1.8+)
- [x] T005 [P] Create `client/androidApp/build.gradle.kts`: apply `com.android.application`, set `compileSdk 35` / `minSdk 26`, add Compose BOM 2024.x, Jetpack Compose UI + Material3, Lifecycle ViewModel + Compose integration, Navigation Compose; add `:shared` as dependency
- [x] T006 [P] Create `client/androidApp/src/main/AndroidManifest.xml`: declare `<uses-permission android:name="android.permission.INTERNET"/>` and `MainActivity` as launcher activity
- [x] T007 [P] Create `server/Dockerfile`: multi-stage â€” Stage 1: `golang:1.23-alpine` builder (`CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server ./cmd/server`); Stage 2: `gcr.io/distroless/static-debian12:nonroot` final image, `COPY --from=builder /server /server`, `EXPOSE 8080`, `CMD ["/server"]`
- [x] T008 [P] Create `server/render.yaml`: `services: [{type: web, name: blind-map-survival, runtime: docker, region: singapore, plan: free, envVars: [{key: PORT, value: "8080"}, {key: LOG_LEVEL, value: info}, {key: MAX_ROOMS, value: "100"}, {key: TURN_SECONDS, value: "30"}, {key: ALLOWED_ORIGINS, value: "*"}]}]`

**Checkpoint**: `cd server && go mod download` succeeds; `cd client && ./gradlew :shared:assemble` completes without error.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared types, protocol DTOs, and client networking layer that EVERY user story depends on. No user story work begins until this phase is complete.

**âš ï¸ CRITICAL**: Complete this entire phase before starting any user story phase.

- [x] T009 Create `server/internal/config/config.go`: `Config` struct fields `Port`, `LogLevel`, `MaxRooms`, `TurnSeconds`, `MapSize`, `AllowedOrigins`; `Load() Config` reads env vars with defaults PORT=8080, LOG_LEVEL=info, MAX_ROOMS=100, TURN_SECONDS=30, MAP_SIZE=20, ALLOWED_ORIGINS=*
- [x] T010 [P] Create `server/internal/game/state.go`: `PlayerID`/`RoomID` (string aliases); `Phase` constants (PhaseLobby/PhaseActive/PhaseEnded); `Direction` constants (N/S/E/W); `Position{X,Y int}`; `ItemKind` constants (bullet/compass); `Item{ID,Kind}`; `Player` struct with all `json:"-"` fields (Pos, StartPos, Inventory, VisitedCells, SkipNextTurn, InfoBlackout) exactly as in `specs/001-blind-map-survival/data-model.md`; `CellKind` constants (empty/bullet/reward/trap/portal_a/portal_b); `Cell{Pos,Kind,PortalID}`; `GameState` struct
- [x] T011 [P] Create `server/internal/protocol/messages.go`: `Envelope{Type string, Ts int64, Data json.RawMessage}`; `JoinData`; `ActionKind` constants (move/pickup/shoot/submit_map); `ActionData{Kind,Direction,ItemID}`; `EventKind` constants (all 8: trap_triggered/reward_activated/player_eliminated/shot_fired/map_submitted/portal_used/clue_received/turn_skipped); `Event{Kind,Payload}`; `WelcomeData`; `LobbyView`; `GameOverData`; `ErrorData`; `ServerShutdownData` â€” matching `specs/001-blind-map-survival/contracts/websocket-protocol.md`
- [x] T012 [P] Create `server/internal/protocol/views.go`: `SelfView`, `OtherPlayerView`, `CellView`, `PlayerView` exactly as in `specs/001-blind-map-survival/data-model.md`; implement `BuildPlayerView(state *game.GameState, playerID game.PlayerID) PlayerView` â€” populates SelfView (own pos/inventory included), OtherPlayerView slice (no pos/inventory), VisibleMap (only cells in player.VisitedCells), respects InfoBlackout flag by omitting clue Events
- [x] T013 Create `client/shared/src/commonMain/kotlin/com/blindmap/protocol/Messages.kt`: `@Serializable data class Envelope(val type: String, val ts: Long, val data: JsonElement)`; `JoinData(roomCode, playerName, clientVersion, playerId: String? = null)`; `ActionData(kind: String, direction: String? = null, itemId: String? = null)` â€” matching Kotlin section of data-model.md
- [x] T014 [P] Create `client/shared/src/commonMain/kotlin/com/blindmap/protocol/Views.kt`: `Position`, `Item`, `SelfView`, `OtherPlayerView`, `CellView`, `PlayerView`, `Event`, `LobbyView`, `WelcomeData`, `GameOverData` â€” all `@Serializable data class`, matching Kotlin section of data-model.md
- [x] T015 [P] Create `client/shared/src/commonMain/kotlin/com/blindmap/state/GameState.kt`: `enum class ConnState { Connecting, Connected, Reconnecting, Failed }`; `enum class GamePhase { Lobby, Active, Ended }`; `data class ClientGameState(roomId, playerId, phase, lobby, game, gameOver, connState, errorMessage)` with defaults as in data-model.md
- [x] T016 Create `client/shared/src/commonMain/kotlin/com/blindmap/net/WebSocketClient.kt`: Ktor WS client; `connect(serverUrl: String, roomCode: String, playerName: String, playerId: String? = null)` opens `$serverUrl?room=$roomCode&name=$playerName`, sends `join` Envelope immediately; exposes `incoming: SharedFlow<Envelope>` by emitting received text frames decoded via `Json.decodeFromString`; `send(envelope: Envelope)` encodes to JSON string and sends as text frame; `close()`
- [x] T017 Create `client/shared/src/commonMain/kotlin/com/blindmap/state/Reducer.kt`: `fun reduce(state: ClientGameState, envelope: Envelope): ClientGameState` â€” pure function; switch on `envelope.type`: `"welcome"` â†’ decode WelcomeData, set playerId + lobby + phase=Lobby; `"lobby_update"` â†’ decode LobbyView, update lobby; `"game_start"` â†’ decode PlayerView, set game + phase=Active; `"turn_result"` â†’ decode PlayerView, update game; `"event"` â†’ decode Event, append to game.events; `"game_over"` â†’ decode GameOverData, set gameOver + phase=Ended; `"error"` â†’ set errorMessage; `"pong"` â†’ no-op; `"server_shutdown"` â†’ set connState=Reconnecting
- [x] T018 Create `client/shared/src/commonMain/kotlin/com/blindmap/viewmodel/GameViewModel.kt`: extends `ViewModel`; holds `WebSocketClient` + `MutableStateFlow<ClientGameState>`; exposes `uiState: StateFlow<ClientGameState>`; `connect(serverUrl, roomCode, playerName)` â€” launches coroutine: calls `wsClient.connect(...)`, collects `wsClient.incoming`, applies `Reducer.reduce` on each envelope, emits to state flow; `sendAction(data: ActionData)` â€” sends `Envelope(type="action", ts=now, data=encode(data))`; `sendJoin(roomCode, playerName)` â€” sends `Envelope(type="join", ...)`

**Checkpoint**: `cd server && go build ./...` compiles clean; `cd client && ./gradlew :shared:assemble` compiles clean with no type errors.

---

## Phase 3: User Story 1 â€” Create and Join a Private Room (P1) ðŸŽ¯ MVP

**Goal**: Players can create a room (unique 6-char code), others join by code, host starts the game; server sends `welcome` + `lobby_update` to all clients; Android lobby screen is functional.

**Independent Test**: `go run ./cmd/server` â†’ wscat joins room TEST01 â†’ receives `welcome` with playerId and lobby; second wscat joins same room â†’ both receive `lobby_update` with 2 players; `/healthz` returns `{"status":"ok"}`.

- [x] T019 [US1] Create `server/internal/game/map.go`: `GenerateMap(mapSize int, rng *rand.Rand) [][]Cell` â€” allocate mapSizeÃ—mapSize CellEmpty grid; place ~10% bullet tiles at random positions; place ~5% reward tiles; place ~3% trap tiles; place 2â€“4 portal pairs (each pair: random non-overlapping positions, same PortalID); helper `randomEmptyCell(grid [][]Cell, rng *rand.Rand) Position`
- [x] T020 [US1] Create `server/internal/hub/hub.go`: `Hub{rooms map[RoomID]*Room, mu sync.RWMutex, cfg *config.Config}`; `NewHub(cfg)`, `Run(ctx context.Context)` (no-op goroutine for future extension); `CreateRoom() (RoomID, *Room, error)` â€” generate 6-char uppercase alphanumeric code via `crypto/rand`, retry on collision, enforce cfg.MaxRooms (return error if at limit), instantiate and start Room goroutine; `GetRoom(id) (*Room, bool)`; `DeleteRoom(id)`
- [x] T021 [US1] Create `server/internal/conn/conn.go`: `Conn{wsConn websocket.Conn, playerName string, writeCh chan []byte}`; `NewConn(wsConn, playerName) *Conn`; `ReadPump(ctx context.Context, roomCh chan<- IncomingMsg)` â€” loop: read text frame, track message count (close conn with 1008 if > 10/s), decode JSON Envelope, push `IncomingMsg{PlayerID, Envelope}` to roomCh; `WritePump(ctx)` â€” drain writeCh, write text frames; `Send(data []byte)` â€” non-blocking push to writeCh (drop if full); `Close()`
- [x] T022 [US1] Create `server/internal/room/room.go`: `Room{state *game.GameState, conns map[game.PlayerID]*conn.Conn, inCh chan IncomingMsg, cfg *config.Config}`; `NewRoom(id game.RoomID, cfg)`, `Run(ctx)` goroutine with `select{case msg := <-inCh: â€¦, case <-ticker.C: â€¦}`; handle `join` message type: validate room not full (8 max, send `error{ROOM_FULL}`), assign PlayerID, create Player, assign host (first joiner), send `welcome` Envelope to new conn, broadcast `lobby_update` to all; handle `leave`/disconnect: remove player, reassign host if needed, broadcast `lobby_update`; stub `startGame()` (to be implemented in Phase 4)
- [x] T023 [US1] Create `server/cmd/server/main.go`: load Config; init slog JSON handler at cfg.LogLevel; create + start Hub; register HTTP handlers: `GET /healthz` â†’ `{"status":"ok"}` JSON; `GET /ws` â†’ validate Origin against cfg.AllowedOrigins (403 if not allowed), parse + validate `room` query param (6-char alphanumeric) and `name` (1â€“20 chars, trimmed), get-or-create room in Hub, upgrade to WebSocket via `websocket.Accept(w, r, opts)`, create Conn, start ReadPump+WritePump goroutines; graceful SIGTERM: `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`, shutdown server within 10s deadline; log startup with port
- [x] T024 [P] [US1] Create `client/androidApp/src/main/res/values/config.xml`: `<string name="server_url">ws://10.0.2.2:8080/ws</string>`
- [x] T025 [P] [US1] Create `client/androidApp/src/main/kotlin/com/blindmap/android/MainActivity.kt`: `ComponentActivity`, `setContent { NavHost(navController, startDestination="lobby") { composable("lobby") { LobbyScreen(vm, nav) }; composable("game") { GameScreen(vm, nav) }; composable("gameover") { GameOverScreen(vm, nav) } } }`; obtain serverUrl from `getString(R.string.server_url)`
- [x] T026 [US1] Create `client/androidApp/src/main/kotlin/com/blindmap/android/ui/LobbyScreen.kt`: Compose screen; `viewModel.uiState.collectAsState()`; player name TextField + room code TextField; "Create Room" button â€” generates random 6-char uppercase code, calls `viewModel.connect(serverUrl, code, name)`; "Join Room" button â€” uses entered code; show players list from `state.lobby?.players`; "Start Game" button (visible when `state.lobby?.isHost == true && players.size >= 2`) â€” calls `viewModel.sendAction(ActionData("start_game"))`; CircularProgressIndicator + "Waking serverâ€¦" text when `state.connState == ConnState.Connecting`; LaunchedEffect to navigate to "game" when `state.phase == GamePhase.Active`

**Checkpoint**: US1 complete â€” start server, two players join, lobby shows both names, host triggers game start; `curl http://localhost:8080/healthz` returns `{"status":"ok"}`.

---

## Phase 4: User Story 2 â€” Take Turns and Navigate the Map (P1)

**Goal**: Active-phase turn loop works â€” Move(N/S/E/W) and Pickup actions resolve; per-player filtered PlayerView sent after each turn; 30s timer auto-skips idle players.

**Independent Test**: Start game with 2 players; Player A sends `{"type":"action","ts":â€¦,"data":{"kind":"move","direction":"N"}}`; A receives `turn_result` with updated pos; B receives `turn_result` with no pos info for A; wait 30s â†’ `turn_skipped` event fires.

- [x] T027 [US2] Create `server/internal/room/turn.go`: `AdvanceTurn(state *game.GameState, playerID game.PlayerID, action *protocol.ActionData) ([]protocol.Event, error)` â€” dispatch on `action.Kind`: `"move"`: validate dir âˆˆ {N,S,E,W} (return `ErrorData{NOT_YOUR_TURN}` / `INVALID_DIRECTION`), compute new pos, validate within [0,mapSize), update `player.Pos`, add to `player.VisitedCells`; `"pickup"`: check cell at player.Pos is CellBullet (return `NOTHING_TO_PICKUP` if not), add `Item{Kind:bullet}` to Inventory (tile stays); return events; `BuildAndBroadcast(state, conns map)`: call `BuildPlayerView` per connected player, send `turn_result` Envelope to each
- [x] T028 [US2] Update `server/internal/room/room.go`: implement `startGame()` â€” seed `crypto/rand` â†’ `math/rand/v2`, call `game.GenerateMap`, place players at distinct random positions (hidden from all), shuffle PlayerIDs into TurnOrder, set Phase=PhaseActive, TurnDeadline=now+TurnSeconds, broadcast `game_start` Envelope (each player gets individual PlayerView); add 100ms ticker to select loop: on tick, if `time.Now().After(state.TurnDeadline)` call `skipCurrentTurn()` (emit `turn_skipped` event, advance TurnOrder index, reset TurnDeadline); on inCh action during PhaseActive: validate it's sender's turn (return `NOT_YOUR_TURN` error if not), call `AdvanceTurn`, call `BuildAndBroadcast`, advance TurnOrder, reset TurnDeadline
- [x] T029 [P] [US2] Update `client/shared/src/commonMain/kotlin/com/blindmap/state/Reducer.kt`: ensure `game_start` sets `state.copy(game = decodePlayerView(data), phase = Active)`; `turn_result` updates `state.copy(game = decodePlayerView(data))`; handle `event` by appending to `state.game?.events` list (immutable copy); handle `error` by setting `state.copy(errorMessage = decoded.message)`
- [x] T030 [US2] Create `client/androidApp/src/main/kotlin/com/blindmap/android/ui/GameScreen.kt`: Compose screen; `val state by viewModel.uiState.collectAsState()`; show turn timer countdown (compute seconds remaining from `state.game?.turnEndsAt`); display visited cells grid (LazyVerticalGrid showing cells from `state.game?.visibleMap`, own position highlighted); action buttons Row (enabled only when `state.game?.currentTurn == state.game?.self?.id`): "â†‘ N", "â†“ S", "â† W", "â†’ E" each calls `viewModel.sendAction(ActionData("move", direction="N"))` etc.; "Pick Up" button (calls `viewModel.sendAction(ActionData("pickup"))`); opponents Column with alive status (alive=âœ“, dead=âœ—); events LazyColumn (last 5 events); `LaunchedEffect(state.phase)` to navigate "gameover" when phase==Ended

**Checkpoint**: US2 complete â€” 2-player turn loop runs; Move/Pickup work; timer auto-skips; no opponent position in turn_result payload.

---

## Phase 5: User Story 3 â€” Collect Items and Use Clues (P2)

**Goal**: All cell types trigger on step: reward tile (3 random effects, repositions), trap tile (5 random penalties), teleport portal (exit partner); compass clue event sent only to target player.

**Independent Test**: Place player on reward tile â†’ one of three effects applied, tile repositions; place on trap tile â†’ one of five penalties applied to that player only; step into portal â†’ exit paired portal location; pick up compass â†’ `clue_received` event seen only by picking player.

- [x] T031 [US3] Create `server/internal/game/items.go`: `ResolveReward(state *GameState, player *Player, rng *rand.Rand) []protocol.Event` â€” pick effect 0-2 (all_positions_revealed | nearest_direction | all_bullet_locations), build Event payload, move reward tile to random empty non-special cell via `randomEmptyCell`; `ResolveTrap(state *GameState, player *Player, rng *rand.Rand) []protocol.Event` â€” pick penalty 0-4 (reveal_position | random_teleport | lose_next_turn | lose_bullet | info_blackout), apply: set player.SkipNextTurn, remove bullet from Inventory, set player.InfoBlackout, or teleport to random empty cell; `ResolvePortal(state *GameState, player *Player) []protocol.Event` â€” find paired portal by PortalID in Grid, set player.Pos to partner cell; `ResolveCompass(state *GameState, player *Player, rng *rand.Rand) protocol.Event` â€” pick clue 0-3 (nearest_direction | own_start_pos | one_other_player_pos | 3x3_surroundings), return private `clue_received` Event
- [x] T032 [US3] Update `server/internal/room/turn.go` in `applyMove()`: after updating player.Pos and VisitedCells, check `state.Grid[newY][newX].Kind`; call `game.ResolveReward` / `game.ResolveTrap` / `game.ResolvePortal` accordingly; accumulate returned events; if trap sets InfoBlackout, the `BuildPlayerView` call in views.go already suppresses clue events for that player (via the InfoBlackout flag check added to T012) and clears the flag afterward
- [x] T033 [P] [US3] Update `client/androidApp/src/main/kotlin/com/blindmap/android/ui/GameScreen.kt`: expand event display to render by kind: `trap_triggered` â†’ show effect name (e.g., "Trap! You lose your next turn"); `reward_activated` â†’ show effect + value (e.g., "Reward! Nearest player: NE"); `clue_received` â†’ show clue prominently in a highlighted card; `portal_used` â†’ show "Teleported!"; show "Turn skipped" disabled-button state when player has SkipNextTurn (infer from turn_skipped event for self)

**Checkpoint**: US3 complete â€” all cell-type effects fire; compass clue verified private (no other player receives it).

---

## Phase 6: User Story 4 â€” Eliminate Opponents via Shooting (P2)

**Goal**: Shoot action consumes bullet, traces ray in direction, instantly eliminates first player in path; game ends with `winReason="last_alive"` when only one player remains.

**Independent Test**: 2-player game; give Player A a bullet (place on bullet tile, Pickup); Player A shoots toward B's position; B receives `player_eliminated` event; A receives `game_over{winner:"Alice", winReason:"last_alive"}`.

- [x] T034 [US4] Update `server/internal/room/turn.go`: add `applyShoot(state *game.GameState, shooter *game.Player, dir game.Direction) ([]protocol.Event, error)` â€” validate shooter has bullet (return `ErrorData{NO_BULLET}` if inventory empty); remove bullet from Inventory; trace ray: from shooter.Pos step in dir until map boundary, check each position for alive player; on first hit: set victim.Alive=false, append `player_eliminated` Event (payload: playerName+byPlayerName) to all-players list; after shoot resolves: count alive players; if 1 remains â†’ set state.Phase=PhaseEnded, state.Winner=&survivorID; wire `"shoot"` ActionKind to `applyShoot` in dispatch switch; return `game_over` Event when winner found
- [x] T035 [US4] Update `server/internal/room/room.go`: after calling AdvanceTurn and broadcasting turn_result, check if returned events include game_over; if so: send `game_over` Envelope to all connected players; schedule `time.AfterFunc(60*time.Second, func(){ hub.DeleteRoom(state.RoomID) })`
- [x] T036 [P] [US4] Update `client/androidApp/src/main/kotlin/com/blindmap/android/ui/GameScreen.kt`: add "Shoot" button visible when `self.inventory.any { it.kind == "bullet" }` and it's player's turn; tapping shows a direction BottomSheet with N/S/E/W options; on selection calls `viewModel.sendAction(ActionData("shoot", direction=chosen))`; handle `player_eliminated` event: cross out that player's name in opponents list; show Snackbar `"${payload.playerName} was eliminated by ${payload.byPlayerName}"`
- [x] T037 [P] [US4] Create `client/androidApp/src/main/kotlin/com/blindmap/android/ui/GameOverScreen.kt`: display `state.gameOver?.winner ?: "No winner"`, localized win reason ("Last player standing" for last_alive / "First to map the entire board" for map_complete); "Play Again" button calls `viewModel.reset()` and navigates back to "lobby"

**Checkpoint**: US4 complete â€” shoot â†’ instant elimination â†’ last-alive game_over fires; eliminated player observes remainder.

---

## Phase 7: User Story 5 â€” Win by Mapping the Entire Board (P3)

**Goal**: Submit Map action validates server-side VisitedCells count; full map â†’ `game_over{winReason:"map_complete"}`; partial map â†’ `MAP_INCOMPLETE` error with remaining count.

**Independent Test**: Start server with `MAP_SIZE=3`; single player visits all 9 cells; sends `submit_map` â†’ receives `game_over{winReason:"map_complete"}`; repeat after visiting only 8 cells â†’ receives `error{code:"MAP_INCOMPLETE", message:"1 cells remain"}`.

- [x] T038 [US5] Update `server/internal/room/turn.go`: add `applySubmitMap(state *game.GameState, player *game.Player) ([]protocol.Event, error)` â€” total = state.MapSize * state.MapSize; if `len(player.VisitedCells) < total`: return `ErrorData{Code:"MAP_INCOMPLETE", Message:fmt.Sprintf("%d cells remain", total-len(player.VisitedCells))}`; else: set state.Phase=PhaseEnded, state.Winner=&player.ID, return `game_over{winner:player.Name, winReason:"map_complete"}` Event; wire `"submit_map"` ActionKind in dispatch switch
- [x] T039 [US5] Update `client/androidApp/src/main/kotlin/com/blindmap/android/ui/GameScreen.kt`: add "Submit Map" button (enabled when it's player's turn); show exploration progress `"${self.visitedCount}/${self.totalCells} cells"` as LinearProgressIndicator + text; handle `error{code:"MAP_INCOMPLETE"}` â†’ Snackbar showing remaining count parsed from message; calls `viewModel.sendAction(ActionData("submit_map"))`

**Checkpoint**: US5 complete â€” map_complete win path verified end-to-end; MAP_INCOMPLETE correctly rejected.

---

## Phase 8: User Story 6 â€” Reconnect After Disconnection (P3)

**Goal**: Disconnected player slot is preserved; they rejoin with original playerId and receive current game state; client uses exponential backoff (1sÃ—2^attempt, cap 30s, max 5 attempts).

**Independent Test**: Start 2-player game; kill Player A network; verify A's turn is auto-skipped on 30s timeout; reconnect A with original playerId â†’ A receives current turn_result and can resume; kill A 5 times â†’ ConnState.Failed shown.

- [x] T040 [US6] Update `server/internal/room/room.go`: on Conn.ReadPump returning (WS disconnect): do NOT remove Player from state; mark `player.LastSeen = time.Now()`; remove only from `conns` map; if disconnected player's turn hits deadline, skip normally; on new `join` message with matching `data.playerId` of existing player: restore new Conn to that Player slot in `conns`, send `welcome` Envelope + current `turn_result` (BuildPlayerView for that player) to reconnected conn; on last-alive check: if 0 alive players remain â†’ emit `game_over{winner:null}` (draw)
- [x] T041 [US6] Create `client/shared/src/commonMain/kotlin/com/blindmap/net/ReconnectManager.kt`: `class ReconnectManager(val wsClient: WebSocketClient)`; `suspend fun reconnect(playerId: String, serverUrl: String, roomCode: String, playerName: String)` â€” loop up to 5 times: delay `min(2.0.pow(attempt).toLong(), 30L) * 1000ms`, call `wsClient.connect(serverUrl, roomCode, playerName, playerId)`, on success return; emit `ConnState.Reconnecting` during loop via shared Flow; emit `ConnState.Failed` after 5 failures
- [x] T042 [US6] Update `client/shared/src/commonMain/kotlin/com/blindmap/net/WebSocketClient.kt`: after connect, launch ping coroutine: send `Envelope(type="ping", ts=now, data=EmptyObject)` every 20s; start `withTimeoutOrNull(10_000L)` awaiting pong; if timeout reached, close WS and invoke `ReconnectManager.reconnect`; expose `onClosed: Flow<Unit>` that emits on any WS close so callers can trigger reconnect independently
- [x] T043 [US6] Update `client/shared/src/commonMain/kotlin/com/blindmap/state/Reducer.kt`: handle `"server_shutdown"` â†’ decode `ServerShutdownData`, set `connState = ConnState.Reconnecting`; add explicit `connState` field transitions: Connectingâ†’Connected on first `welcome`, Reconnectingâ†’Connected on rejoin `welcome`, Failed stays Failed
- [x] T044 [US6] Update `client/shared/src/commonMain/kotlin/com/blindmap/viewmodel/GameViewModel.kt`: collect `wsClient.onClosed`; on close, launch `reconnectManager.reconnect(state.playerId, ...)` and emit `ConnState.Reconnecting` updates to uiState; on `ConnState.Failed`, emit final failure state
- [x] T045 [US6] Update `client/androidApp/src/main/kotlin/com/blindmap/android/ui/GameScreen.kt`: show `"Reconnectingâ€¦ (attempt N/5)"` animated Banner when `state.connState == ConnState.Reconnecting`; show full-screen error Card `"Connection failed â€” return to lobby"` with a Back button when `state.connState == ConnState.Failed`; hide reconnect UI and resume normal game UI when `state.connState == ConnState.Connected`

**Checkpoint**: US6 complete â€” disconnect/reconnect cycle verified; exponential delays visible in logs; 5-attempt failure triggers Failed state.

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Input validation, structured logging, deployment verification, and go/Kotlin build health checks.

- [x] T046 Add `log/slog` JSON structured logging throughout `server/internal/hub/hub.go` and `server/internal/room/room.go`: Info on room created/destroyed and player joined/left/eliminated; Info on game start/end with winner; Debug on each turn resolved (playerID, action kind, duration); Error on all error paths â€” all guarded by the configured LogLevel
- [x] T047 [P] Enforce rate limiting + origin check in `server/internal/conn/conn.go`: in ReadPump, track per-second message count with a rolling counter reset each second; close conn with status 1008 (policy violation) if count exceeds 10; add `validateOrigin(r *http.Request, allowed string) bool` helper used in the `/ws` handler in main.go
- [x] T048 [P] Implement server-side pong reply in `server/internal/room/room.go`: when inCh delivers an Envelope with type="ping", respond immediately with `{"type":"pong","ts":<now>,"data":{}}` to that Conn only
- [x] T049 Run `cd server && go vet ./...` and fix all reported issues before marking complete
- [x] T050 Run `cd server && go test -race -timeout 60s ./...` and confirm no data races or failures
- [x] T051 [P] Run `cd client && ./gradlew :shared:test` and confirm all Kotlin shared module tests pass
- [x] T052 [P] Build Docker image and verify size: `docker build -t blind-map-survival:local server/`; confirm size is under 20 MB with `docker image inspect blind-map-survival:local --format "{{.Size}}"` (< 20971520 bytes); fix Dockerfile if over limit
- [x] T053 Validate Render.com deployment: push branch to remote, confirm `https://<app>.onrender.com/healthz` returns `{"status":"ok"}` within 60s (accounting for cold-start wake); verify all env vars are applied via Render dashboard

**Checkpoint**: All 9 phases complete â€” game fully playable, Docker image < 20 MB, deployed to Render, all constitution rules met.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies â€” start immediately
- **Phase 2 (Foundational)**: Requires Phase 1 â€” **BLOCKS all user story phases**
- **Phase 3 (US1)**: Requires Phase 2
- **Phase 4 (US2)**: Requires Phase 3 (cannot take turns without a joined room + WebSocket)
- **Phase 5 (US3)**: Requires Phase 4 (items are activated via the move turn action)
- **Phase 6 (US4)**: Requires Phase 4 (shoot is a turn action); can run in parallel with Phase 5
- **Phase 7 (US5)**: Requires Phase 4 (submit_map is a turn action); can run in parallel with Phases 5â€“6
- **Phase 8 (US6)**: Requires Phase 3 (slot preservation is in room logic); independent of Phases 4â€“7
- **Phase 9 (Polish)**: Requires all desired user stories complete

### User Story Dependencies

- **US1 (P1)**: No US dependencies â€” first story after Foundational
- **US2 (P1)**: Depends on US1 (turns require an active room and connected players)
- **US3 (P2)**: Depends on US2 (items trigger during the move turn action)
- **US4 (P2)**: Depends on US2 (shoot is a turn action); independent of US3
- **US5 (P3)**: Depends on US2 (submit_map is a turn action); independent of US3, US4
- **US6 (P3)**: Depends on US1 (player slot lives in Room); independent of US2â€“US5

### Parallel Opportunities

- Phase 1: T002â€“T008 all parallelizable once T001 (directories) is complete
- Phase 2: T010, T011, T012 [P] server types; T014, T015 [P] Kotlin types; T016â€“T018 sequential (each depends on prior types)
- Phase 3: T024, T025 [P] (different resource/Kotlin files) while T019â€“T023 are server-side
- Phase 4: T029 [P] (Reducer update) independent of T027 (turn.go server logic)
- Phase 5: T033 [P] (Android UI) can start once T031's event types are defined
- Phase 6: T036, T037 [P] (Android side) once T034 (server shoot logic) is done
- Phase 8: T041, T042, T043 [P] (different files: ReconnectManager, WebSocketClient, Reducer)
- Phase 9: T047, T048, T050, T051, T052 all [P]

---

## Parallel Example: Phase 2 Foundational

```bash
# These files share no dependencies â€” run simultaneously:
Task T010: server/internal/game/state.go       (Go types)
Task T011: server/internal/protocol/messages.go (Go DTOs)
Task T012: server/internal/protocol/views.go   (Go views)
Task T013: client/.../protocol/Messages.kt     (Kotlin DTOs)
Task T014: client/.../protocol/Views.kt        (Kotlin views)
Task T015: client/.../state/GameState.kt       (Kotlin state)
```

## Parallel Example: User Story 4 (Shoot)

```bash
# Server logic and Android UI have no file overlap â€” run simultaneously after T027 exists:
Task T034: server/internal/room/turn.go        (shoot logic)
Task T036: androidApp/.../ui/GameScreen.kt     (shoot UI)
Task T037: androidApp/.../ui/GameOverScreen.kt (game over screen)
```

---

## Implementation Strategy

### MVP First (US1 + US2 â€” Phases 1â€“4)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (**blocks all stories**)
3. Complete Phase 3: US1 â€” room creation and joining
4. **Validate**: wscat joins, `welcome` + `lobby_update` received; `/healthz` returns ok
5. Complete Phase 4: US2 â€” turn navigation (Move + Pickup)
6. **Validate**: 2-player turn loop, timer auto-skip, no position leaks in payloads
7. Run Phase 9 T052â€“T053: Docker build + Render deploy
8. **Ship MVP** â€” playable on Android against Render.com server

### Incremental Delivery

| After phase | Deliverable | Validate |
|---|---|---|
| Phase 2 | Skeleton compiles | `go build ./...` + `gradlew :shared:assemble` |
| Phase 3 | Lobby works | wscat create/join, lobby_update visible |
| Phase 4 | Playable MVP | 2-player turn loop + Android client |
| Phase 5 | Items + clues | All cell types, private clues verified |
| Phase 6 | Combat | Shoot â†’ elimination â†’ last-alive win |
| Phase 7 | Alt win path | Map submission win |
| Phase 8 | Production-ready | Reconnect under mobile network conditions |
| Phase 9 | Deployed | Render healthz + Docker < 20 MB |

### Constitution Compliance

- **Principle I**: All planned files â‰¤ 500 lines (verified in plan.md â€” largest file GameScreen.kt ~380 lines)
- **Principle II**: Every type, behavior, and rule traces to `blind_map_survival_tech_spec.docx` via spec.md and research.md; request approval before any deviation
- **Principle III**: Commit with `add|fix|delete: <work>` after each checkpoint (â‰¥ 100 lines changed)

---

## Notes

- `[P]` = safe to run in parallel with other tasks in the same phase (different files, no incomplete dependency)
- `[USN]` traces the task to User Story N in `specs/001-blind-map-survival/spec.md`
- All GameState mutations happen inside the Room goroutine â€” no locks needed on game structs (research.md D-002, D-004)
- PlayerView sent to client X MUST NEVER include position/inventory of other players (FR-024, data-model.md filtering rule)
- Use `MAP_SIZE=3` env var for fast US5 testing (9 cells to visit instead of 400)
- Commit after each phase checkpoint: `add: phase N <description>` (Constitution Principle III)

