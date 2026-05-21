#!/usr/bin/env python3
"""
Blind Map Survival — full-feature test runner.

Connects Alice (tester), Bot1 and Bot2 to a local server using the debug map.
Alice navigates to every special cell type, shoots Bot2, tests chat, pause/resume,
and finally submits the map. All output goes to stdout + test_output.txt.

Usage:
    python test_runner.py [server_url]
    (defaults to ws://localhost:8080/ws)
"""

import asyncio
import json
import sys
import time
import os
from collections import deque
from datetime import datetime

try:
    import websockets
except ImportError:
    print("ERROR: install 'websockets' first:  pip install websockets")
    sys.exit(1)

SERVER = sys.argv[1] if len(sys.argv) > 1 else "ws://localhost:8080/ws"
ROOM   = "DBGMAP"
LOG    = "test_output.txt"

# ── Debug map layout (12×12) ───────────────────────────────────────────────────
# Matches BuildDebugMap() in server/internal/game/map.go
DEBUG_RAW = [
    "WWWWWWWWWWWW",  # row 0
    "W..BRTT....W",  # row 1  bullet@3, reward@4, trap@5, trap@6
    "W.T.TRT....W",  # row 2  trap@2, trap@4, reward@5, trap@6
    "W..R.T.....W",  # row 3  reward@3, trap@5
    "W.AZ.......W",  # row 4  portalA@2, portalB@3
    "W..........W",
    "W..........W",
    "W..........W",
    "W..........W",
    "W..........W",
    "W..........W",  # row 10  Bot1@(1,10)
    "WWWWWWWWWWWW",  # row 11
]
MAP_SIZE = 12

def parse_debug_map():
    grid = {}
    for y, row in enumerate(DEBUG_RAW):
        for x, ch in enumerate(row):
            grid[(x, y)] = ch
    return grid

DEBUG_GRID = parse_debug_map()

def cell_at(x, y):
    return DEBUG_GRID.get((x, y), 'W')

def is_passable(x, y):
    return cell_at(x, y) != 'W'

def find_path(start, goal):
    """BFS on the debug map. Returns list of directions ['N','S','E','W']."""
    if start == goal:
        return []
    q = deque([(start, [])])
    visited = {start}
    while q:
        (cx, cy), path = q.popleft()
        for dx, dy, d in [(1,0,'E'),(-1,0,'W'),(0,1,'S'),(0,-1,'N')]:
            nx, ny = cx+dx, cy+dy
            if (nx, ny) in visited or not is_passable(nx, ny):
                continue
            np = path + [d]
            if (nx, ny) == goal:
                return np
            visited.add((nx, ny))
            q.append(((nx, ny), np))
    return []  # unreachable

# All wall positions for correct map submit
DEBUG_WALLS = [
    {"x": x, "y": y}
    for y in range(MAP_SIZE) for x in range(MAP_SIZE)
    if cell_at(x, y) == 'W'
]

# ── Logging ────────────────────────────────────────────────────────────────────
_log_file = None

def log(msg: str):
    ts = datetime.now().strftime("%H:%M:%S.%f")[:-3]
    line = f"[{ts}] {msg}"
    print(line)
    if _log_file:
        _log_file.write(line + "\n")
        _log_file.flush()

# ── Wire helpers ───────────────────────────────────────────────────────────────
def envelope(type_: str, data: dict) -> str:
    return json.dumps({"type": type_, "ts": int(time.time() * 1000), "data": data})

def join_msg(room, name):
    return envelope("join", {"roomCode": room, "playerName": name})

def action(kind, **kw):
    return envelope("action", {"kind": kind, **kw})

def chat(text):
    return envelope("chat", {"text": text})

# ── Bot player ─────────────────────────────────────────────────────────────────
class Bot:
    def __init__(self, name, ws):
        self.name = name
        self.ws = ws
        self.pid = None
        self.done = False

    async def run(self):
        try:
            async for raw in self.ws:
                msg = json.loads(raw)
                t = msg["type"]
                if t == "welcome":
                    self.pid = msg["data"]["playerId"]
                    log(f"  [{self.name}] id={self.pid}")
                elif t == "turn_result":
                    if msg["data"].get("currentTurn") == self.pid:
                        await self.ws.send(action("pass"))
                elif t == "game_over":
                    self.done = True
                    return
        except Exception as e:
            if not self.done:
                log(f"  [{self.name}] gone: {e}")

# ── Alice test orchestrator ────────────────────────────────────────────────────
class Alice:
    def __init__(self, ws):
        self.ws = ws
        self.pid = None
        self.current_turn = None
        self.pos = None        # actual (x, y) from server
        self.turn_num = 0
        self.done = False
        self._fut = None
        self._loop = asyncio.get_event_loop()

    async def run(self):
        try:
            async for raw in self.ws:
                msg = json.loads(raw)
                self._on(msg["type"], msg)
                if self.done:
                    return
        except Exception as e:
            if not self.done:
                log(f"  [Alice] ws: {e}")

    def _on(self, t, msg):
        data = msg.get("data", {})
        if t == "welcome":
            self.pid = data["playerId"]
            log(f"  [Alice] id={self.pid}")
        elif t in ("game_start", "turn_result"):
            self.current_turn = data.get("currentTurn")
            self.turn_num = data.get("turn", self.turn_num)
            self_data = data.get("self", {})
            self.pos = (self_data.get("pos", {}).get("x"),
                        self_data.get("pos", {}).get("y"))
            if t == "game_start":
                log(f"  [Alice] game_start t={self.turn_num} pos={self.pos}")
            for e in (data.get("events") or []):
                log(f"  [Alice] EVENT {e['kind']}: {json.dumps(e.get('payload',{}), ensure_ascii=False)}")
            self._resolve(data)
        elif t == "error":
            log(f"  [Alice] ERROR: {data}")
            self._resolve(data)
        elif t == "chat_msg":
            log(f"  [Alice] CHAT {data.get('senderName')}: {data.get('text')}")
        elif t == "game_over":
            log(f"  [Alice] GAME OVER winner={data.get('winner')} reason={data.get('winReason')}")
            self.done = True
            self._resolve(data)

    def _resolve(self, data):
        if self._fut and not self._fut.done():
            self._fut.set_result(data)

    async def _wait(self, timeout=20.0):
        f = self._loop.create_future()
        self._fut = f
        return await asyncio.wait_for(f, timeout=timeout)

    async def wait_my_turn(self, timeout=30.0):
        deadline = time.time() + timeout
        while self.current_turn != self.pid:
            if self.done or time.time() > deadline:
                return
            await self._wait(timeout=5.0)

    async def send(self, description, payload, wait_turn=True):
        log(f"\n>>> [Alice] {description}  (pos={self.pos})")
        await asyncio.sleep(0.18)
        await self.ws.send(payload)
        await self._wait()
        if wait_turn and not self.done:
            await self.wait_my_turn()
        log(f"      → pos={self.pos}")

    async def navigate_to(self, tx, ty, label=""):
        """Walk from current position to (tx, ty) using BFS on the debug map."""
        path = find_path(self.pos, (tx, ty))
        if not path:
            log(f"  [Alice] WARNING: no path to ({tx},{ty}) {label} from {self.pos}")
            return
        log(f"  [Alice] navigate {self.pos}→({tx},{ty}) {label}  steps={path}")
        for d in path:
            names = {"N": "up", "S": "down", "E": "right", "W": "left"}
            await self.send(f"move {names[d]} toward ({tx},{ty})", action("move", direction=d))
            if self.done:
                return

    async def go_and_step(self, tx, ty, label):
        """Navigate to the cell ADJACENT to (tx,ty) then step on it."""
        if self.done:
            return
        # First navigate to (tx,ty) directly (BFS handles the final step)
        await self.navigate_to(tx, ty, label)


# ── Main ──────────────────────────────────────────────────────────────────────
async def main():
    global _log_file
    _log_file = open(LOG, "w", encoding="utf-8")
    log(f"=== Blind Map Survival Test Runner ===")
    log(f"Server : {SERVER}")
    log(f"Room   : {ROOM}")
    log(f"Log    : {os.path.abspath(LOG)}")
    log(f"Walls  : {len(DEBUG_WALLS)} cells to submit")
    log("")

    url = lambda n: f"{SERVER}?room={ROOM}&name={n}"

    async with (
        websockets.connect(url("Alice")) as aws,
        websockets.connect(url("Bot1"))  as b1ws,
        websockets.connect(url("Bot2"))  as b2ws,
    ):
        # ── Join ──────────────────────────────────────────────────────
        log("── JOIN ────────────────────────────────────────────────────")
        alice = Alice(aws)
        await aws.send(join_msg(ROOM, "Alice"))
        await asyncio.sleep(0.2)
        await b1ws.send(join_msg(ROOM, "Bot1"))
        await b2ws.send(join_msg(ROOM, "Bot2"))
        await asyncio.sleep(0.1)

        bot1 = Bot("Bot1", b1ws)
        bot2 = Bot("Bot2", b2ws)
        t_bot1 = asyncio.create_task(bot1.run())
        t_bot2 = asyncio.create_task(bot2.run())

        # ── Start game ────────────────────────────────────────────────
        log("\n── START GAME (debug map, 15s turns) ───────────────────────")
        await aws.send(action("start_game", mapSize=12, turnSeconds=15, debugMap=True))
        t_alice = asyncio.create_task(alice.run())

        # wait for game_start
        f = alice._loop.create_future()
        alice._fut = f
        await asyncio.wait_for(f, timeout=10.0)
        await alice.wait_my_turn()
        log(f"\n  Alice at {alice.pos}  (expected (1,1))")

        # ── BULLET ───────────────────────────────────────────────────
        log("\n── CELL: BULLET (3,1) ───────────────────────────────────────")
        await alice.go_and_step(3, 1, "bullet")

        # ── REWARD #1 ────────────────────────────────────────────────
        log("\n── CELL: REWARD (4,1) ───────────────────────────────────────")
        await alice.go_and_step(4, 1, "reward#1")

        # ── SHOOT Bot2 ────────────────────────────────────────────────
        log("\n── SHOOT: Bot2 at (10,1) ────────────────────────────────────")
        # Navigate to (9,1) then shoot east
        await alice.navigate_to(9, 1, "pre-shoot position")
        await alice.send("SHOOT E → Bot2@(10,1)", action("shoot", direction="E"))

        # ── TRAPS ────────────────────────────────────────────────────
        log("\n── CELL: TRAP (5,1) ─────────────────────────────────────────")
        await alice.go_and_step(5, 1, "trap1")

        log("\n── CELL: TRAP (6,1) ─────────────────────────────────────────")
        await alice.go_and_step(6, 1, "trap2")

        log("\n── CELL: REWARD (5,2) ───────────────────────────────────────")
        await alice.go_and_step(5, 2, "reward#2")

        log("\n── CELL: TRAP (2,2) ─────────────────────────────────────────")
        await alice.go_and_step(2, 2, "trap3")

        log("\n── CELL: TRAP (4,2) ─────────────────────────────────────────")
        await alice.go_and_step(4, 2, "trap4")

        log("\n── CELL: TRAP (6,2) ─────────────────────────────────────────")
        await alice.go_and_step(6, 2, "trap5")

        log("\n── CELL: REWARD (3,3) ───────────────────────────────────────")
        await alice.go_and_step(3, 3, "reward#3")

        log("\n── CELL: TRAP (5,3) ─────────────────────────────────────────")
        await alice.go_and_step(5, 3, "trap6")

        # ── PORTAL ───────────────────────────────────────────────────
        log("\n── CELL: PORTAL A (2,4) → teleports to B (3,4) ────────────")
        await alice.go_and_step(2, 4, "portalA")

        # ── CHAT ─────────────────────────────────────────────────────
        log("\n── CHAT TEST ────────────────────────────────────────────────")
        log(">>> [Alice] sending chat")
        await aws.send(chat("Hello from Alice test script!"))
        await asyncio.sleep(0.5)
        await b1ws.send(chat("Bot1 says hi!"))
        await asyncio.sleep(0.5)

        # ── PAUSE / RESUME ────────────────────────────────────────────
        log("\n── PAUSE / RESUME TEST ──────────────────────────────────────")
        log(">>> pause")
        await aws.send(action("pause"))
        await asyncio.sleep(0.4)
        log(">>> resume")
        await aws.send(action("resume"))
        await asyncio.sleep(0.4)

        # ── MAP SUBMIT wrong ──────────────────────────────────────────
        log("\n── MAP SUBMIT: WRONG ────────────────────────────────────────")
        await alice.wait_my_turn()
        await alice.send(
            "submit_map WRONG (3 random non-wall cells)",
            action("submit_map", walls=[{"x":1,"y":1},{"x":2,"y":2},{"x":3,"y":3}]),
            wait_turn=True,
        )

        # ── MAP SUBMIT correct ────────────────────────────────────────
        log(f"\n── MAP SUBMIT: CORRECT ({len(DEBUG_WALLS)} walls) ──────────────────────")
        await asyncio.sleep(0.18)
        await aws.send(action("submit_map", walls=DEBUG_WALLS))
        try:
            result = await alice._wait(timeout=10.0)
            evts = result.get("events", [])
            for e in evts:
                log(f"  [Alice] EVENT {e['kind']}: {json.dumps(e.get('payload',{}), ensure_ascii=False)}")
        except asyncio.TimeoutError:
            log("  [Alice] timeout waiting for submit result")

        # ── GAME OVER ─────────────────────────────────────────────────
        if not alice.done:
            log("\n── WAITING FOR GAME OVER ────────────────────────────────────")
            try:
                await asyncio.wait_for(t_alice, timeout=12.0)
            except asyncio.TimeoutError:
                log("  [Alice] no game_over — map may have wrong cells; check log")

        t_bot1.cancel(); t_bot2.cancel(); t_alice.cancel()

    log("\n=== TEST COMPLETE ===")
    log(f"Full log → {os.path.abspath(LOG)}")
    _log_file.close()


if __name__ == "__main__":
    asyncio.run(main())
