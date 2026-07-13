#!/usr/bin/env python3
"""
Blind Map Survival — terminal client for Linux/Ubuntu
Usage: python3 play.py  [--url wss://... --name Bob --room ABC123]

Requires: pip install websockets
"""

import asyncio
import json
import sys
import time
import random
import string
import argparse
import termios
import tty
import threading

try:
    import websockets
except ImportError:
    print("Missing dependency. Install with:  pip install websockets")
    sys.exit(1)

# ── helpers ──────────────────────────────────────────────────────────────────

def now_ms():
    return int(time.time() * 1000)

def envelope(msg_type, data):
    return json.dumps({"type": msg_type, "ts": now_ms(), "data": data})

def action(data):
    return envelope("action", data)

def chat_envelope(text: str) -> str:
    return envelope("chat", {"text": text})

def rand_room():
    chars = string.ascii_uppercase + string.digits
    return "".join(random.choices(chars, k=6))

def getch():
    """Read a single keypress without echo (Linux)."""
    fd = sys.stdin.fileno()
    old = termios.tcgetattr(fd)
    try:
        tty.setraw(fd)
        ch = sys.stdin.read(1)
        if ch == "\x1b":
            ch2 = sys.stdin.read(1)
            if ch2 == "[":
                ch3 = sys.stdin.read(1)
                return {"A": "UP", "B": "DOWN", "C": "RIGHT", "D": "LEFT"}.get(ch3, ch3)
            return ch2
    finally:
        termios.tcsetattr(fd, termios.TCSADRAIN, old)
    return ch

def readline_input(prompt: str) -> str:
    """Temporarily restore cooked mode to read a full line."""
    fd = sys.stdin.fileno()
    old = termios.tcgetattr(fd)
    try:
        termios.tcsetattr(fd, termios.TCSADRAIN, old)
        return input(prompt)
    except (EOFError, KeyboardInterrupt):
        return ""
    finally:
        termios.tcsetattr(fd, termios.TCSADRAIN, old)

def fmt_chat_ts(ms: int) -> str:
    import datetime
    dt = datetime.datetime.fromtimestamp(ms / 1000)
    return dt.strftime("%H:%M")

SEP = "─" * 48

# ── event formatting ──────────────────────────────────────────────────────────

def fmt_event(ev):
    kind = ev.get("kind", "")
    p = ev.get("payload") or {}

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
        }.get(bt, bt)
        return f"{name} - moved {dirn} - ok - {detail}"

    if kind == "trap_triggered":
        eff = p.get("effect", "")
        if eff == "reveal_position":
            name = p.get("playerName", "?")
            pos  = p.get("pos", {})
            return f"trap - {name}'s position revealed: ({pos.get('x','?')},{pos.get('y','?')})"
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
            if locs:
                coords = ", ".join(f"({loc.get('x','?')},{loc.get('y','?')})" for loc in locs)
                return f"reward - all bullet tiles revealed\n    positions: {coords}"
            return "reward - all bullet tiles revealed (none on map)"
        if eff == "all_positions_revealed":
            positions = p.get("positions") or []
            if positions:
                coords = ", ".join(
                    f"{pos.get('name','?')}({pos.get('pos',{}).get('x','?')},{pos.get('pos',{}).get('y','?')})"
                    for pos in positions
                )
                return f"reward - all positions revealed\n    positions: {coords}"
            return "reward - all positions revealed"
        return f"reward - {eff}"

    if kind == "player_eliminated":
        victim = p.get("playerName", "?")
        by = p.get("byPlayerName")
        return f"{by} shot {victim} - eliminated" if by else f"{victim} - eliminated"

    if kind == "shot_fired":
        by = p.get("byPlayerName", "?")
        return f"{by} - fired {p.get('direction', '')}"

    if kind == "map_submitted":
        name = p.get("playerName", "?")
        if p.get("correct"):
            return f"{name} - submitted map - correct! win!"
        return f"{name} - submitted map - {p.get('wrong', 0)} wrong ({p.get('submitsLeft', '?')} left)"

    if kind == "portal_used":
        return f"{p.get('playerName', '?')} - teleported through a portal"

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
            grid = {c["rel"]: c["kind"] for c in cells}
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
            return f"{name} is at ({pos.get('x','?')},{pos.get('y','?')})"
        if itype == "own_start":
            pos = p.get("pos", {})
            return f"your start pos: ({pos.get('x','?')},{pos.get('y','?')})"
        return f"info revealed: {itype}"

    return kind

# ── action log & map stats ────────────────────────────────────────────────────

action_log  = []   # list of {"turn": int, "text": str}
map_counts  = {}   # {"blank": n, "wall": n, ...}
map_paused  = False

def log_events(turn_num, events):
    for ev in events:
        action_log.append({"turn": turn_num, "text": fmt_event(ev)})

def show_action_log():
    recent = action_log[-20:]
    print()
    print(SEP)
    print(f"  {'Turn':<6}  Event")
    print(SEP)
    for e in recent:
        lines = e['text'].split('\n')
        print(f"  {e['turn']:<6}  {lines[0]}")
        for cont in lines[1:]:
            print(f"  {'':6}  {cont}")
    if not recent:
        print("  (no events yet)")
    print(SEP)

def show_map_info(map_size):
    print()
    print(SEP)
    if not map_counts:
        print("  (map info not available yet)")
    else:
        print(f"  Map info ({map_size}x{map_size})")
        print(SEP)
        print(f"  {'Type':<10} Count")
        print(SEP)
        for kind in ("blank", "wall", "bullet", "reward", "trap", "info", "portal"):
            print(f"  {kind:<10} {map_counts.get(kind, 0)}")
    print(SEP)

# ── map painter ───────────────────────────────────────────────────────────────

def render_map(size, walls):
    hdr = "    " + " ".join(f"{x}" for x in range(size))
    print(hdr)
    print("   +" + "─" * (size * 2 - 1) + "+")
    for y in range(size):
        row = " ".join("█" if (x, y) in walls else "·" for x in range(size))
        print(f"  {y}|{row}|")
    print("   +" + "─" * (size * 2 - 1) + "+")
    print(f"   {len(walls)} walls marked")
    print("   X Y=toggle  ok=submit  clear=reset  q=cancel")

def map_paint_session(size, walls_in):
    """Blocking map-paint session (runs in thread executor). Returns (walls_set, submit)."""
    walls = set(walls_in)
    render_map(size, walls)
    while True:
        try:
            line = readline_input("Map> ").strip()
        except (EOFError, KeyboardInterrupt):
            return walls, False
        if line.lower() in ("q", "quit", "cancel"):
            return walls, False
        if line.lower() in ("ok", "submit", "sub"):
            return walls, True
        if line.lower() == "clear":
            walls.clear()
            render_map(size, walls)
            continue
        if line.lower() == "show":
            render_map(size, walls)
            continue
        parts = line.split()
        if len(parts) == 2:
            try:
                x, y = int(parts[0]), int(parts[1])
                if 0 <= x < size and 0 <= y < size:
                    key = (x, y)
                    if key in walls:
                        walls.discard(key)
                        print(f"   ({x},{y}) cleared")
                    else:
                        walls.add(key)
                        print(f"   ({x},{y}) marked wall")
                    render_map(size, walls)
                else:
                    print(f"   Use 0-{size-1} for x and y")
            except ValueError:
                print("   Type: X Y  or  ok  or  q")
        else:
            print("   Type: X Y  or  ok  or  clear  or  q")
    return walls, False

# ── state display ─────────────────────────────────────────────────────────────

def show_state(data, my_id):
    global map_counts, map_paused

    is_mine = data.get("currentTurn") == my_id
    ends_at = data.get("turnEndsAt", 0)
    secs    = max(0, int((ends_at - now_ms()) / 1000))
    turn    = data.get("turn", 0)

    # track map metadata
    ms = data.get("mapStats") or {}
    if ms.get("counts"):
        map_counts = ms["counts"]
    if "paused" in data:
        map_paused = bool(data["paused"])

    others_list = data.get("others") or []
    turn_label  = "YOUR TURN" if is_mine else (
        next((o["name"] for o in others_list if o["id"] == data.get("currentTurn")), "?") + "'s turn"
    )

    print()
    print(SEP)
    if map_paused:
        print("  *** GAME PAUSED — press P to resume ***")
        print(SEP)
    print(f"  Turn {turn}  |  {turn_label}  |  {secs}s left")

    events = data.get("events") or []
    if events:
        log_events(turn, events)
        print(SEP)
        for ev in events:
            for line in fmt_event(ev).split('\n'):
                print(f"  > {line}")

    print(SEP)
    print("  [W/A/S/D] Move  [F] Shoot  [M] Map  [N] Info  [L] Log  [T] Chat  [P] Pause  [Q] Quit")

# ── async game loop ───────────────────────────────────────────────────────────

async def run(url, name, room, map_size_arg, turn_secs):
    wsurl = f"{url}?room={room}&name={name}"

    ws = None
    waited = 0
    attempt = 0
    print("Connecting", end="", flush=True)
    while waited < 60:
        try:
            ws = await websockets.connect(wsurl, open_timeout=10)
            break
        except Exception:
            attempt += 1
            delay = min(2 ** attempt, 10)
            print(".", end="", flush=True)
            await asyncio.sleep(delay)
            waited += delay
    print()
    if ws is None:
        print("Could not reach server after 60s.")
        return
    if attempt > 0:
        print(f"Server woke up after {waited}s.")
    print("Connected.")

    my_id      = ""
    shoot_mode = False
    map_size   = map_size_arg
    map_walls  = set()
    loop       = asyncio.get_event_loop()

    key_queue: asyncio.Queue = asyncio.Queue()

    def read_keys():
        while True:
            try:
                ch = getch()
                asyncio.run_coroutine_threadsafe(key_queue.put(ch), loop)
            except Exception:
                break

    t = threading.Thread(target=read_keys, daemon=True)
    t.start()

    async def handle_input():
        nonlocal shoot_mode, map_size, map_walls

        while True:
            ch = await key_queue.get()
            upper = ch.upper() if isinstance(ch, str) else ch

            if shoot_mode:
                shoot_mode = False
                dirmap = {"W": "N", "UP": "N", "S": "S", "DOWN": "S",
                          "A": "W", "LEFT": "W", "D": "E", "RIGHT": "E"}
                dirn = dirmap.get(upper)
                if dirn:
                    await ws.send(action({"kind": "shoot", "direction": dirn}))
                else:
                    print("(shoot cancelled)")
                continue

            if upper in ("W", "UP"):
                await ws.send(action({"kind": "move", "direction": "N"}))
            elif upper in ("S", "DOWN"):
                await ws.send(action({"kind": "move", "direction": "S"}))
            elif upper in ("A", "LEFT"):
                await ws.send(action({"kind": "move", "direction": "W"}))
            elif upper in ("D", "RIGHT"):
                await ws.send(action({"kind": "move", "direction": "E"}))
            elif upper == "G":
                await ws.send(action({"kind": "start_game", "mapSize": map_size_arg, "turnSeconds": turn_secs}))
            elif upper == "B":
                lvl = await loop.run_in_executor(
                    None, readline_input, "Bot (easy/medium/hard, 'remove' drops last, Enter=medium): "
                )
                lvl = (lvl or "").strip().lower() or "medium"
                if lvl in ("remove", "r"):
                    await ws.send(action({"kind": "remove_bot"}))
                elif lvl in ("easy", "medium", "hard", "e", "m", "h"):
                    full = {"e": "easy", "m": "medium", "h": "hard"}.get(lvl, lvl)
                    await ws.send(action({"kind": "add_bot", "difficulty": full}))
                else:
                    print("(unknown bot difficulty)")
            elif upper == "F":
                shoot_mode = True
                print("Shoot direction: W/A/S/D")
            elif upper == "M":
                new_walls, do_submit = await loop.run_in_executor(
                    None, map_paint_session, map_size, map_walls
                )
                map_walls = new_walls
                if do_submit:
                    wall_list = [{"x": x, "y": y} for (x, y) in map_walls]
                    await ws.send(action({"kind": "submit_map", "walls": wall_list}))
                    print("(map submitted)")
            elif upper == "N":
                show_map_info(map_size)
            elif upper == "L":
                show_action_log()
            elif upper == "P":
                if map_paused:
                    await ws.send(action({"kind": "resume"}))
                    print("(resume sent)")
                else:
                    await ws.send(action({"kind": "pause"}))
                    print("(pause sent)")
            elif upper == "T":
                text = await loop.run_in_executor(None, readline_input, "Chat: ")
                text = text.strip()
                if text:
                    await ws.send(chat_envelope(text))
            elif upper in ("Q", "\x03"):
                await ws.close()
                return

    async def handle_messages():
        nonlocal my_id, map_size
        async for raw in ws:
            try:
                msg   = json.loads(raw)
                mtype = msg.get("type")
                data  = msg.get("data", {})
                if mtype == "welcome":
                    my_id = data.get("playerId", "")
                    rs    = data.get("roomState", {})
                    print(f"Joined room {rs.get('roomCode','?')}")
                elif mtype == "lobby_update":
                    players = ", ".join(data.get("players", []))
                    is_host = data.get("isHost", False)
                    print(f"Lobby: {players}{' (host)' if is_host else ''}")
                elif mtype in ("game_start", "turn_result"):
                    ms = (data.get("mapStats") or {}).get("mapSize")
                    if ms:
                        map_size = ms
                    show_state(data, my_id)
                elif mtype == "error":
                    code = data.get("code", "")
                    friendly = {
                        "INVALID_DIRECTION": "Wall! Can't move that way.",
                        "NOTHING_TO_PICKUP": "Nothing to pick up here.",
                        "NO_BULLET":         "No bullet in inventory.",
                        "MAP_INCOMPLETE":    f"Map incomplete — {data.get('message','')}",
                    }.get(code, data.get("message", code))
                    print(f"! {friendly}")
                elif mtype == "game_over":
                    winner = data.get("winner")
                    reason = {"last_alive": "last player standing",
                              "map_complete": "mapped the entire board"}.get(
                        data.get("winReason", ""), data.get("winReason", ""))
                    print()
                    if winner:
                        print(f"=== GAME OVER — {winner} wins ({reason}) ===")
                    else:
                        print("=== GAME OVER — No winner ===")
                elif mtype == "chat_msg":
                    print(f"  [CHAT] {data.get('senderName','?')}: {data.get('text','')}")
                elif mtype == "chat_history":
                    pass  # silently ignore — only show live messages
                elif mtype == "server_shutdown":
                    print("Server is restarting…")
                elif mtype == "pong":
                    pass
            except Exception:
                print(f"< {raw}")

    await asyncio.gather(handle_messages(), handle_input())

# ── entry point ───────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="Blind Map Survival — terminal client")
    parser.add_argument("--url",  default="wss://maptracker-c68n.onrender.com/ws")
    parser.add_argument("--name", default="")
    parser.add_argument("--room", default="")
    args = parser.parse_args()

    name = args.name or input("Your name: ").strip()
    map_size  = 10
    turn_secs = 30
    room = args.room
    if not room:
        choice = input("C=Create room   J=Join room: ").strip().upper()
        if choice == "C":
            room = rand_room()
            print(f"Room code: {room}  (share this, or start solo)")
            ms = input("Map size (4-20, Enter=10): ").strip()
            if ms.isdigit() and 4 <= int(ms) <= 20:
                map_size = int(ms)
            ts = input("Turn time in seconds (10-120, Enter=30): ").strip()
            if ts.isdigit() and 10 <= int(ts) <= 120:
                turn_secs = int(ts)
            print(f"Settings: {map_size}x{map_size} map, {turn_secs}s turns")
        else:
            room = input("Room code: ").strip().upper()

    print()
    print("Controls:")
    print("  W/A/S/D or Arrow keys = Move")
    print("  G                     = Start game (host only)")
    print("  B                     = Add/remove bot (host, in lobby)")
    print("  M                     = Paint map / submit")
    print("  F then W/A/S/D        = Shoot")
    print("  N                     = Map info (cell counts)")
    print("  L                     = Show last 20 events")
    print("  T                     = Chat")
    print("  P                     = Pause / Resume")
    print("  Q                     = Quit")
    print()

    asyncio.run(run(args.url, name, room, map_size, turn_secs))
    print("Disconnected.")

if __name__ == "__main__":
    main()
