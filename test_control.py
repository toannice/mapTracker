#!/usr/bin/env python3
"""
Control Test Runner — 4×4 fixed map, two scenarios.

Map layout:
  col→  0    1    2    3
row↓
  0     me   T    T    T
  1     I    R    R    R
  2     I    I    .    W
  3     Bot2 .    Bot1 B

Scenario 1 (room CTRL01):
  Me@(0,0) Bot1@(2,3) Bot2@(0,3)
  Me moves: D,D,D,S,A,A,A,S,D,D,D,S,D → shoot W → submit map.
  Bots stay still (pass every turn).
  Log: test_control_s1.txt  (Me's POV — exactly what you see in terminal)

Scenario 2 (room CTRL02):
  Me@(2,3) Bot1@(0,0) Bot2@(0,3)
  Bot1 executes the same sequence. Me stays still (passes every turn).
  Log: test_control_s2.txt  (Me's POV — other player's perspective)

Usage:
  python test_control.py [ws://localhost:8080/ws]
"""

import asyncio
import json
import sys
import time
import os
from datetime import datetime

try:
    import websockets
except ImportError:
    print("ERROR: pip install websockets")
    sys.exit(1)

SERVER = sys.argv[1] if len(sys.argv) > 1 else "ws://localhost:8080/ws"
SEP    = "─" * 48

# ── wire helpers ──────────────────────────────────────────────────────────────

def now_ms():
    return int(time.time() * 1000)

def envelope(t, data):
    return json.dumps({"type": t, "ts": now_ms(), "data": data})

def join_msg(room, name):
    return envelope("join", {"roomCode": room, "playerName": name})

def action(kind, **kw):
    return envelope("action", {"kind": kind, **kw})

# ── event formatting (mirrors play.py exactly) ────────────────────────────────

def fmt_event(ev):
    kind = ev.get("kind", "")
    p    = ev.get("payload") or {}

    if kind == "player_moved":
        name = p.get("playerName", "?")
        dirn = p.get("direction", "?")
        if not p.get("success"):
            return f"{name} - moved {dirn} - wall"
        bt = p.get("blockType", "blank")
        detail = {
            "blank":  "empty",
            "bullet": "bullet full" if p.get("bulletFull") else "picked up bullet",
            "reward": "reward",
            "trap":   "trap",
            "portal": "portal",
            "info":   "info",
        }.get(bt, bt)
        return f"{name} - moved {dirn} - ok - {detail}"

    if kind == "trap_triggered":
        eff = p.get("effect", "")
        if eff == "reveal_position":
            pos = p.get("pos", {})
            return f"trap - position revealed ({pos.get('x','?')},{pos.get('y','?')})"
        return {
            "random_teleport": "trap - random teleport",
            "lose_next_turn":  "trap - lose next turn",
            "lose_bullet":     "trap - lost bullet",
        }.get(eff, f"trap - {eff}")

    if kind == "reward_activated":
        eff = p.get("effect", "")
        if eff == "nearest_direction":
            return f"reward - nearest player is {p.get('direction','?')}"
        if eff == "all_bullet_locations":
            locs = p.get("locations") or []
            loc_str = "".join(f"[{loc.get('y','?')}-{loc.get('x','?')}]" for loc in locs)
            return f"reward - all bullet tiles revealed {loc_str}"
        if eff == "all_positions_revealed":
            positions = p.get("positions") or []
            parts = [
                f"{pos.get('name','?')} at {pos.get('pos',{}).get('y','?')}-{pos.get('pos',{}).get('x','?')}"
                for pos in positions
            ]
            return "reward - all positions revealed: " + "  ".join(parts) if parts else "reward - all positions revealed"
        return f"reward - {eff}"

    if kind == "player_eliminated":
        victim = p.get("playerName", "?")
        by     = p.get("byPlayerName")
        return f"{by} shot {victim} - eliminated" if by else f"{victim} - eliminated"

    if kind == "shot_fired":
        by = p.get("byPlayerName", "?")
        return f"{by} - fired {p.get('direction', '')}"

    if kind == "map_submitted":
        name = p.get("playerName", "?")
        if p.get("correct"):
            return f"{name} - submitted map - correct! win!"
        return f"{name} - submitted map - {p.get('wrong', 0)} wrong ({p.get('submitsLeft', '?')} left)"

    if kind == "portal_used":        return "portal - teleported"
    if kind == "you_were_eliminated":
        by = p.get("byPlayerName")
        return f"YOU were eliminated by {by}" if by else "YOU were eliminated"
    if kind == "turn_skipped":
        reason = p.get("reason", "")
        suffix = " (timeout)" if reason == "timeout" else " (trap)" if reason == "trap_effect" else ""
        return f"{p.get('playerName','?')} - turn skipped{suffix}"

    if kind == "info_revealed":
        itype = p.get("type", "")
        if itype == "surroundings_3x3":
            cells = p.get("cells", [])
            grid  = {c["rel"]: c["kind"] for c in cells}
            def sym(r, c):
                k = grid.get(f"{r}-{c}", "wall")
                return {"empty":".", "wall":"W", "trap":"T", "reward":"R",
                        "bullet":"B", "info":"I", "portal_a":"A", "portal_b":"Z"}.get(k, "?")
            lines = [
                f"  {sym(0,0)} {sym(0,1)} {sym(0,2)}",
                f"  {sym(1,0)} @ {sym(1,2)}",
                f"  {sym(2,0)} {sym(2,1)} {sym(2,2)}",
            ]
            return "3x3 surroundings:\n" + "\n".join(lines)
        if itype == "player_position":
            name = p.get("playerName","?")
            pos  = p.get("pos", {})
            return f"{name} at {pos.get('y','?')}-{pos.get('x','?')}"
        if itype == "own_start":
            pos = p.get("pos", {})
            return f"you start at {pos.get('y','?')}-{pos.get('x','?')}"
        return f"info revealed: {itype}"

    return kind

# ── logger ────────────────────────────────────────────────────────────────────

class Logger:
    def __init__(self, path):
        self._f = open(path, "w", encoding="utf-8")

    def log(self, msg=""):
        ts   = datetime.now().strftime("%H:%M:%S.%f")[:-3]
        line = f"[{ts}] {msg}" if msg else ""
        print(line)
        self._f.write(line + "\n")
        self._f.flush()

    def close(self):
        self._f.close()

# ── bot: passes every turn ────────────────────────────────────────────────────

class StillBot:
    def __init__(self, name, ws):
        self.name = name
        self.ws   = ws
        self.pid  = None
        self.done = False

    async def run(self):
        try:
            async for raw in self.ws:
                msg = json.loads(raw)
                t   = msg["type"]
                if t == "welcome":
                    self.pid = msg["data"]["playerId"]
                elif t == "turn_result":
                    if msg["data"].get("currentTurn") == self.pid:
                        await self.ws.send(action("pass"))
                elif t == "game_over":
                    self.done = True
                    return
        except Exception:
            pass

# ── bot: alternates moving right then left each of its turns ─────────────────

class AlternatingBot:
    def __init__(self, name, ws):
        self.name  = name
        self.ws    = ws
        self.pid   = None
        self.done  = False
        self._turn = 0   # even → move E, odd → move W

    async def run(self):
        try:
            async for raw in self.ws:
                msg = json.loads(raw)
                t   = msg["type"]
                if t == "welcome":
                    self.pid = msg["data"]["playerId"]
                elif t == "turn_result":
                    if msg["data"].get("currentTurn") == self.pid:
                        direction = "E" if self._turn % 2 == 0 else "W"
                        self._turn += 1
                        await self.ws.send(action("move", direction=direction))
                elif t == "game_over":
                    self.done = True
                    return
        except Exception:
            pass

# ── main player: logs + optionally auto-passes ────────────────────────────────

class Player:
    """
    Logs everything Me receives in play.py format.
    auto_pass=True: automatically passes whenever it's Me's turn (scenario 2 observer).
    auto_pass=False: moves are driven externally via send_action() (scenario 1).
    """
    def __init__(self, name, ws, logger, auto_pass=False):
        self.name      = name
        self.ws        = ws
        self.log       = logger
        self.auto_pass = auto_pass
        self.pid       = None
        self.turn      = 0
        self.pos       = None
        self._last_ct  = None   # last known currentTurn — used by wait_my_turn
        self.done      = False
        self._fut      = None
        self._loop     = asyncio.get_event_loop()

    async def run(self):
        try:
            async for raw in self.ws:
                msg = json.loads(raw)
                await self._on(msg["type"], msg)
                if self.done:
                    return
        except Exception as e:
            if not self.done:
                self.log.log(f"  [{self.name}] ws error: {e}")

    async def _on(self, t, msg):
        data = msg.get("data", {})
        if t == "welcome":
            self.pid = data["playerId"]
            self.log.log(f"  [{self.name}] joined  id={self.pid}")

        elif t in ("game_start", "turn_result"):
            self.turn      = data.get("turn", self.turn)
            ct             = data.get("currentTurn")
            self._last_ct  = ct
            secs           = max(0, int((data.get("turnEndsAt", 0) - now_ms()) / 1000))
            self_d         = data.get("self", {})
            self.pos       = (self_d.get("pos", {}).get("x"), self_d.get("pos", {}).get("y"))
            inv            = self_d.get("inventory", [])
            bullets        = sum(1 for i in inv if i.get("kind") == "bullet")

            others    = data.get("others") or []
            turn_name = self.name if ct == self.pid else next(
                (o["name"] for o in others if o["id"] == ct), "?")
            whose     = "YOUR TURN" if ct == self.pid else f"{turn_name}'s turn"

            self.log.log()
            self.log.log(SEP)
            if data.get("paused"):
                self.log.log("  *** GAME PAUSED ***")
                self.log.log(SEP)
            self.log.log(f"  Turn {self.turn}  |  {whose}  |  {secs}s left")
            self.log.log(f"  pos=({self.pos[0]},{self.pos[1]})  bullets={bullets}")

            events = data.get("events") or []
            if events:
                self.log.log(SEP)
                for ev in events:
                    for line in fmt_event(ev).split("\n"):
                        self.log.log(f"  > {line}")
            self.log.log(SEP)
            self.log.log("  [W/A/S/D] Move  [F] Shoot  [M] Map  [N] Info  [L] Log  [T] Chat  [P] Pause  [Q] Quit")

            if t == "game_start":
                self.log.log(f"  [{self.name}] GAME START  pos={self.pos}")

            # Auto-pass if observer mode and it's this player's turn
            if self.auto_pass and ct == self.pid and not self.done:
                await asyncio.sleep(0.1)
                await self.ws.send(action("pass"))

            self._resolve(data)

        elif t == "error":
            self.log.log(f"  [{self.name}] ERROR: {data.get('code')} — {data.get('message')}")
            self._resolve(data)

        elif t == "game_over":
            winner = data.get("winner")
            reason = {"last_alive": "last player standing",
                      "map_complete": "mapped the entire board"}.get(
                data.get("winReason",""), data.get("winReason",""))
            self.log.log()
            if winner:
                self.log.log(f"=== GAME OVER — {winner} wins ({reason}) ===")
            else:
                self.log.log("=== GAME OVER — No winner ===")
            self.done = True
            self._resolve(data)

    def _resolve(self, data):
        if self._fut and not self._fut.done():
            self._fut.set_result(data)

    async def _wait(self, timeout=25.0):
        f = self._loop.create_future()
        self._fut = f
        return await asyncio.wait_for(f, timeout=timeout)

    async def wait_my_turn(self, timeout=40.0):
        """Wait until it's this player's turn. Returns immediately if already our turn."""
        if self._last_ct == self.pid:
            return
        deadline = time.time() + timeout
        while not self.done and time.time() < deadline:
            try:
                data = await self._wait(timeout=5.0)
                if data.get("currentTurn") == self.pid:
                    return
            except asyncio.TimeoutError:
                if self._last_ct == self.pid:
                    return

    async def send_action(self, description, payload):
        self.log.log(f"\n>>> [{self.name}] {description}  pos=({self.pos[0]},{self.pos[1]})")
        await asyncio.sleep(0.15)
        await self.ws.send(payload)
        try:
            await self._wait(timeout=20.0)
        except asyncio.TimeoutError:
            self.log.log(f"  [{self.name}] timeout waiting for response")
        if not self.done:
            await self.wait_my_turn()
        self.log.log(f"      → pos=({self.pos[0]},{self.pos[1]})")

# ── moving bot (scenario 2: Bot1 executes the full sequence) ─────────────────

DIR_LABEL = {"E":"D(east)", "W":"A(west)", "S":"S(south)", "N":"W(north)"}

MOVE_SEQUENCE = [
    ("move","E"), ("move","E"), ("move","E"),
    ("move","S"),
    ("move","W"), ("move","W"), ("move","W"),
    ("move","S"),
    ("move","E"), ("move","E"), ("move","E"),
    ("move","S"),
    ("move","E"),
]

CONTROL_WALLS = [{"x": 3, "y": 2}]


class MovingBot:
    """Executes MOVE_SEQUENCE then shoot W then submit map."""
    def __init__(self, name, ws, logger):
        self.name       = name
        self.ws         = ws
        self.log        = logger
        self.pid        = None
        self.done       = False
        self._step      = 0
        self._shot      = False
        self._submitted = False

    async def run(self):
        try:
            async for raw in self.ws:
                msg  = json.loads(raw)
                t    = msg["type"]
                data = msg.get("data", {})
                if t == "welcome":
                    self.pid = data["playerId"]
                elif t in ("game_start", "turn_result"):
                    ct = data.get("currentTurn")
                    # Log private events only Bot1 receives (not visible to observer)
                    for ev in (data.get("events") or []):
                        if ev.get("kind") == "info_revealed":
                            for line in fmt_event(ev).split("\n"):
                                self.log.log(f"  [Bot1 private] > {line}")
                    if ct == self.pid and not self.done:
                        await asyncio.sleep(0.15)
                        await self._do_next()
                elif t == "game_over":
                    self.done = True
                    return
        except Exception:
            pass

    async def _do_next(self):
        if self._step < len(MOVE_SEQUENCE):
            kind, direction = MOVE_SEQUENCE[self._step]
            self._step += 1
            label = DIR_LABEL.get(direction, direction)
            self.log.log(f"\n>>> [Bot1] {label}")
            await self.ws.send(action(kind, direction=direction))
        elif not self._shot:
            self._shot = True
            self.log.log(f"\n>>> [Bot1] shoot W (west/left)")
            await self.ws.send(action("shoot", direction="W"))
        elif not self._submitted:
            self._submitted = True
            self.log.log(f"\n>>> [Bot1] submit map {CONTROL_WALLS}")
            await self.ws.send(action("submit_map", walls=CONTROL_WALLS))
        else:
            await self.ws.send(action("pass"))

# ── scenario runner ───────────────────────────────────────────────────────────

async def run_scenario(scenario, room_code, log_path):
    logger = Logger(log_path)
    logger.log(f"=== CONTROL TEST — Scenario {scenario} ===")
    logger.log(f"Server : {SERVER}")
    logger.log(f"Room   : {room_code}")
    logger.log(f"Log    : {os.path.abspath(log_path)}")
    if scenario == 1:
        logger.log("Setup  : Me@(0,0) moves. Bot1@(2,3) stays still. Bot2@(0,3) alternates E/W.")
        logger.log("Seq    : D,D,D,S,A,A,A,S,D,D,D,S,D → shoot W → submit map")
    else:
        logger.log("Setup  : Me@(2,3) stays still. Bot1@(0,0) executes same sequence. Bot2@(0,3) alternates E/W.")
        logger.log("Goal   : Capture what Me (observer) sees + Bot1's private info events.")
    logger.log("")

    url = lambda n: f"{SERVER}?room={room_code}&name={n}"

    async with (
        websockets.connect(url("Me"),   open_timeout=15) as me_ws,
        websockets.connect(url("Bot1"), open_timeout=15) as b1ws,
        websockets.connect(url("Bot2"), open_timeout=15) as b2ws,
    ):
        logger.log("── JOIN ────────────────────────────────────────────────────")
        await me_ws.send(join_msg(room_code, "Me"))
        await asyncio.sleep(0.2)
        await b1ws.send(join_msg(room_code, "Bot1"))
        await b2ws.send(join_msg(room_code, "Bot2"))
        await asyncio.sleep(0.1)

        # In scenario 1: Me moves, bots stay still
        # In scenario 2: Me auto-passes (observer), Bot1 moves, Bot2 stays still
        auto_pass_me = (scenario == 2)
        me   = Player("Me", me_ws, logger, auto_pass=auto_pass_me)
        bot2 = AlternatingBot("Bot2", b2ws)

        if scenario == 1:
            bot1_agent = StillBot("Bot1", b1ws)
        else:
            bot1_agent = MovingBot("Bot1", b1ws, logger)

        t_me   = asyncio.create_task(me.run())
        t_bot1 = asyncio.create_task(bot1_agent.run())
        t_bot2 = asyncio.create_task(bot2.run())

        logger.log(f"\n── START GAME (control map scenario {scenario}, 60s turns) ──")
        await me_ws.send(action("start_game",
                                controlMap=True,
                                controlScenario=scenario,
                                turnSeconds=60))

        # Wait for game_start (Me's run() handles logging it)
        loop = asyncio.get_event_loop()
        f = loop.create_future()
        me._fut = f
        try:
            await asyncio.wait_for(f, timeout=10.0)
        except asyncio.TimeoutError:
            logger.log("ERROR: game did not start within 10s")
            t_me.cancel(); t_bot1.cancel(); t_bot2.cancel()
            logger.close()
            return

        if scenario == 1:
            # Scenario 1: Me executes the full move sequence
            # wait_my_turn returns immediately since game_start already tells us it's Me's turn
            await me.wait_my_turn()

            logger.log(f"\n── MOVE SEQUENCE (Me's POV) ─────────────────────────────")
            for move_kind, direction in MOVE_SEQUENCE:
                if me.done:
                    break
                label = DIR_LABEL.get(direction, direction)
                await me.send_action(f"move {label}", action(move_kind, direction=direction))

            if not me.done:
                logger.log(f"\n── SHOOT West (A) ───────────────────────────────────────")
                await me.send_action("shoot W (west/left)", action("shoot", direction="W"))

            if not me.done:
                logger.log(f"\n── SUBMIT MAP ───────────────────────────────────────────")
                logger.log(f"  Submitting walls: {CONTROL_WALLS}")
                await me_ws.send(action("submit_map", walls=CONTROL_WALLS))
                try:
                    await me._wait(timeout=10.0)
                except asyncio.TimeoutError:
                    logger.log("  timeout waiting for submit result")

        # Wait for game over (both scenarios)
        if not me.done:
            logger.log("\n── WAITING FOR GAME OVER ────────────────────────────────")
            try:
                await asyncio.wait_for(t_me, timeout=300.0)
            except asyncio.TimeoutError:
                logger.log("  timed out waiting for game over")

        t_me.cancel(); t_bot1.cancel(); t_bot2.cancel()

    logger.log(f"\n=== SCENARIO {scenario} COMPLETE ===")
    logger.log(f"Log → {os.path.abspath(log_path)}")
    logger.close()


# ── entry point ───────────────────────────────────────────────────────────────

async def main():
    print(f"Control Test Runner")
    print(f"Server: {SERVER}")
    print()

    print("Running Scenario 1 — Me's POV (Me moves, bots still)...")
    await run_scenario(1, "CTRL01", "test_control_s1.txt")
    await asyncio.sleep(1.5)

    print("\nRunning Scenario 2 — Observer POV (Bot1 moves, Me stays still)...")
    await run_scenario(2, "CTRL02", "test_control_s2.txt")

    print("\nDone.")
    print("  Scenario 1 (Me's POV)       → test_control_s1.txt")
    print("  Scenario 2 (Observer's POV) → test_control_s2.txt")


if __name__ == "__main__":
    asyncio.run(main())
