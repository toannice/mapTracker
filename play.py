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
import os
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
        # handle escape sequences (arrow keys → ESC [ A/B/C/D)
        if ch == "\x1b":
            ch2 = sys.stdin.read(1)
            if ch2 == "[":
                ch3 = sys.stdin.read(1)
                return {"A": "UP", "B": "DOWN", "C": "RIGHT", "D": "LEFT"}.get(ch3, ch3)
            return ch2
    finally:
        termios.tcsetattr(fd, termios.TCSADRAIN, old)
    return ch

def readline_chat() -> str:
    """Temporarily restore cooked mode to read a full chat line."""
    fd = sys.stdin.fileno()
    old = termios.tcgetattr(fd)
    try:
        termios.tcsetattr(fd, termios.TCSADRAIN, old)
        return input("Chat: ")
    except (EOFError, KeyboardInterrupt):
        return ""
    finally:
        termios.tcsetattr(fd, termios.TCSADRAIN, old)

def fmt_chat_ts(ms: int) -> str:
    import datetime
    dt = datetime.datetime.fromtimestamp(ms / 1000)
    return dt.strftime("%H:%M")

SEP = "─" * 44

def fmt_event(ev):
    kind = ev.get("kind", "")
    p = ev.get("payload") or {}
    if kind == "player_moved":
        name = p.get("playerName", "?")
        dirn = p.get("direction", "?")
        if not p.get("success"):
            return f"{name} — moved {dirn} — hit a wall"
        bt = p.get("blockType", "blank")
        if bt == "bullet":
            return f"{name} — moved {dirn} — {'bullet tile (full)' if p.get('bulletFull') else 'picked up a bullet'}"
        if bt == "reward":  return f"{name} — moved {dirn} — stepped on a reward"
        if bt == "trap":    return f"{name} — moved {dirn} — triggered a trap"
        if bt == "portal":  return f"{name} — moved {dirn} — entered a portal"
        return f"{name} — moved {dirn}"
    if kind == "trap_triggered":
        eff = p.get("effect", "")
        if eff == "reveal_position":
            pos = p.get("pos", {})
            return f"Trap — position revealed at ({pos.get('x','?')},{pos.get('y','?')})"
        if eff == "random_teleport": return "Trap — a player was teleported randomly"
        if eff == "lose_next_turn":  return "Trap — a player loses their next turn"
        if eff == "lose_bullet":     return "Trap — a player lost their bullet"
        if eff == "info_blackout":   return "Trap — a player's info is blacked out"
        return "Trap triggered"
    if kind == "reward_activated":
        eff = p.get("effect", "")
        if eff == "all_positions_revealed": return "Reward — everyone's positions revealed"
        if eff == "nearest_direction":      return f"Reward — nearest player is {p.get('direction','?')}"
        if eff == "all_bullet_locations":   return "Reward — all bullet tiles revealed"
        return "Reward activated"
    if kind == "player_eliminated":
        victim = p.get("playerName", "?")
        by     = p.get("byPlayerName")
        return f"{by} — shot — {victim} eliminated" if by else f"{victim} — eliminated"
    if kind == "shot_fired":
        by = p.get("byPlayerName", "?")
        dirn = p.get("direction", "")
        return f"{by} — fired {dirn}".rstrip()
    if kind == "map_submitted":
        name = p.get("playerName", "?")
        if p.get("correct"): return f"{name} — submitted map — correct! Win!"
        return f"{name} — submitted map — {p.get('wrong',0)} cell(s) wrong"
    if kind == "portal_used":   return "Portal — a player teleported"
    if kind == "clue_received": return "Clue received"
    if kind == "turn_skipped":  return f"{p.get('playerName','?')} — turn skipped"
    return kind.replace("_", " ")

def show_state(data, my_id, player_name):
    self_   = data.get("self", {})
    is_mine = data.get("currentTurn") == my_id
    ends_at = data.get("turnEndsAt", 0)
    secs    = max(0, int((ends_at - now_ms()) / 1000))

    inv = ", ".join(i["kind"] for i in (self_.get("inventory") or [])) or "empty"
    others_list = data.get("others") or []
    others = "  ".join(
        ("✓" if o.get("alive") else "✗") + " " + o["name"]
        for o in others_list
    ) or "none"

    turn_label = "YOUR TURN" if is_mine else (
        next((o["name"] for o in others_list if o["id"] == data.get("currentTurn")), "?") + "'s turn"
    )

    print()
    print(SEP)
    print(f"  Turn {data.get('turn',0)}  |  {turn_label}  |  {secs}s left")
    print(SEP)
    print(f"  Explored: {self_.get('visitedCount',0)}/{self_.get('totalCells',0)}")
    print(f"  Items: {inv}")
    print(f"  Others: {others}")

    events = data.get("events") or []
    if events:
        print(SEP)
        for ev in events:
            print(f"  > {fmt_event(ev)}")

    print(SEP)
    if is_mine:
        print("  [W/A/S/D] Move   [M] Submit map   [F] Shoot   [G] Start   [Q] Quit")


# ── async WebSocket game loop ─────────────────────────────────────────────────

async def run(url, name, room, map_size, turn_secs):
    wsurl = f"{url}?room={room}&name={name}"

    # cold-start retry
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
    loop       = asyncio.get_event_loop()

    # keyboard input thread → queue
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
        nonlocal shoot_mode
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
                await ws.send(action({"kind": "start_game", "mapSize": map_size, "turnSeconds": turn_secs}))
            elif upper == "M":
                await ws.send(action({"kind": "submit_map", "walls": []}))
            elif upper == "F":
                shoot_mode = True
                print("Shoot direction: W/A/S/D")
            elif upper == "T":
                text = await loop.run_in_executor(None, readline_chat)
                text = text.strip()
                if text:
                    await ws.send(chat_envelope(text))
            elif upper in ("Q", "\x03"):
                await ws.close()
                return

    async def handle_messages():
        nonlocal my_id
        async for raw in ws:
            try:
                msg  = json.loads(raw)
                mtype = msg.get("type")
                data = msg.get("data", {})
                if mtype == "welcome":
                    my_id = data.get("playerId", "")
                    rs    = data.get("roomState", {})
                    print(f"Joined room {rs.get('roomCode','?')}")
                elif mtype == "lobby_update":
                    players = ", ".join(data.get("players", []))
                    is_host = data.get("isHost", False)
                    print(f"Lobby: {players}{' (you are host)' if is_host else ''}")
                elif mtype in ("game_start", "turn_result"):
                    show_state(data, my_id, name)
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
                    t = fmt_chat_ts(data.get("ts", 0))
                    print(f"[{t}] {data.get('senderName','?')}: {data.get('text','')}")
                elif mtype == "chat_history":
                    msgs = data.get("messages") or []
                    if msgs:
                        print("── chat history ──")
                        for m in msgs:
                            t = fmt_chat_ts(m.get("ts", 0))
                            print(f"[{t}] {m.get('senderName','?')}: {m.get('text','')}")
                        print("──────────────────")
                elif mtype == "server_shutdown":
                    print("Server is restarting…")
                elif mtype == "pong":
                    pass
            except Exception as e:
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
    map_size  = 20
    turn_secs = 30
    room = args.room
    if not room:
        choice = input("C=Create room   J=Join room: ").strip().upper()
        if choice == "C":
            room = rand_room()
            print(f"Room code: {room}  (share this, or start solo)")
            ms = input("Map size (8-40, Enter=20): ").strip()
            if ms.isdigit() and 8 <= int(ms) <= 40:
                map_size = int(ms)
            ts = input("Turn time in seconds (10-120, Enter=30): ").strip()
            if ts.isdigit() and 10 <= int(ts) <= 120:
                turn_secs = int(ts)
            print(f"Settings: {map_size}x{map_size} map, {turn_secs}s turns")
        else:
            room = input("Room code: ").strip().upper()

    print()
    print("Controls:")
    print("  W/A/S/D or Arrow keys = Move  (bullets auto-picked up on move)")
    print("  G                     = Start game (host only)")
    print("  M                     = Submit map (win condition)")
    print("  F then W/A/S/D        = Shoot in direction")
    print("  T                     = Type a chat message")
    print("  Q                     = Quit")
    print()

    asyncio.run(run(args.url, name, room, map_size, turn_secs))
    print("Disconnected.")


if __name__ == "__main__":
    main()
