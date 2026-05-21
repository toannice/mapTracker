# Blind Map Survival — terminal client
# Usage: .\play.ps1  (or  .\play.ps1 -Url wss://... -Name Bob -Room ABC123)
param(
    [string]$Url  = "wss://maptracker-c68n.onrender.com/ws",
    [string]$Name = "",
    [string]$Room = ""
)

if (!$Name) { $Name = Read-Host "Your name" }
$MapSize    = 10
$TurnSecs   = 30
$IsCreating = $false
if (!$Room) {
    $c = Read-Host "C=Create room   J=Join room"
    if ($c -ieq 'C') {
        $IsCreating = $true
        $Room = -join ((65..90 + 48..57) | Get-Random -Count 6 | ForEach-Object { [char]$_ })
        Write-Host "Room code: $Room  (share this with others, or start solo)" -ForegroundColor Cyan
        $ms = Read-Host "Map size (4-20, Enter=10)"
        if ($ms -match '^\d+$' -and [int]$ms -ge 4 -and [int]$ms -le 20) { $MapSize = [int]$ms }
        $ts = Read-Host "Turn time in seconds (10-120, Enter=30)"
        if ($ts -match '^\d+$' -and [int]$ts -ge 10 -and [int]$ts -le 120) { $TurnSecs = [int]$ts }
        Write-Host "Settings: ${MapSize}x${MapSize} map, ${TurnSecs}s turns  (wall density randomised each game)" -ForegroundColor DarkCyan
    } else {
        $Room = (Read-Host "Room code").ToUpper()
    }
}

[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.SecurityProtocolType]::Tls12

$ws  = [System.Net.WebSockets.ClientWebSocket]::new()
$ct  = [System.Threading.CancellationToken]::None
$enc = [System.Text.Encoding]::UTF8

$wsUrl = "$Url`?room=$Room&name=$([Uri]::EscapeDataString($Name))"

# Cold-start retry loop — Render free tier can take up to 60s to wake
$connected = $false
$attempt   = 0
$maxWait   = 60
$waited    = 0
Write-Host "Connecting" -NoNewline -ForegroundColor DarkGray
while (-not $connected -and $waited -lt $maxWait) {
    $ws = [System.Net.WebSockets.ClientWebSocket]::new()
    try {
        [void]$ws.ConnectAsync([Uri]$wsUrl, $ct).GetAwaiter().GetResult()
        $connected = $true
    } catch {
        $attempt++
        $delay = [Math]::Min([Math]::Pow(2, $attempt), 10)
        Write-Host "." -NoNewline -ForegroundColor DarkGray
        Start-Sleep -Seconds $delay
        $waited += $delay
    }
}
Write-Host ""
if (-not $connected) {
    Write-Host "Could not reach server after ${maxWait}s. Is the URL correct?" -ForegroundColor Red
    exit 1
}
if ($attempt -gt 0) {
    Write-Host "Server woke up after ${waited}s." -ForegroundColor Yellow
}
Write-Host "Connected." -ForegroundColor Green

function Send-Json([string]$json) {
    $b = $enc.GetBytes($json)
    try {
        [void]$ws.SendAsync([ArraySegment[byte]]$b, 'Text', $true, $ct).GetAwaiter().GetResult()
    } catch {
        Write-Host "Send error: $_" -ForegroundColor Red
    }
}
function Now { [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds() }
function Action([string]$dataJson) {
    '{"type":"action","ts":' + (Now) + ',"data":' + $dataJson + '}'
}
function ChatEnvelope([string]$text) {
    '{"type":"chat","ts":' + (Now) + ',"data":{"text":' + ($text | ConvertTo-Json) + '}}'
}
function Format-ChatTs([long]$ms) {
    [DateTimeOffset]::FromUnixTimeMilliseconds($ms).LocalDateTime.ToString("HH:mm")
}

# Background receive loop
$shared = [hashtable]::Synchronized(@{
    ws          = $ws
    running     = $true
    myId        = ''
    playerName  = $Name
    mapSize     = $MapSize
    turnSecs    = $TurnSecs
    actionLog   = [System.Collections.Generic.List[string]]::new()
    mapWalls    = [System.Collections.Generic.HashSet[string]]::new()
    gameMapSize = $MapSize
    paused      = $false
    mapCounts   = $null
})
$rs = [System.Management.Automation.Runspaces.RunspaceFactory]::CreateRunspace()
$rs.Open()
$rs.SessionStateProxy.SetVariable('shared', $shared)
$rs.SessionStateProxy.SetVariable('enc', $enc)
$ps = [powershell]::Create()
$ps.Runspace = $rs
[void]$ps.AddScript({
    function Format-Event($ev) {
        $p = $ev.payload
        switch ($ev.kind) {
            'player_moved' {
                $name = if ($p -and $p.playerName) { $p.playerName } else { '?' }
                $dirn = if ($p -and $p.direction)  { $p.direction  } else { '?' }
                if (-not $p.success) { return "$name - moved $dirn - wall" }
                $bt = if ($p.blockType) { $p.blockType } else { 'blank' }
                $detail = switch ($bt) {
                    'blank'  { 'empty' }
                    'bullet' { if ($p.bulletFull) { 'bullet full' } else { 'picked up bullet' } }
                    'reward' { 'reward' }
                    'trap'   { 'trap' }
                    'portal' { 'portal' }
                    default  { $bt }
                }
                return "$name - moved $dirn - ok - $detail"
            }
            'trap_triggered' {
                $eff = if ($p -and $p.effect) { $p.effect } else { '' }
                switch ($eff) {
                    'reveal_position' {
                        $x = if ($p.pos) { $p.pos.x } else { '?' }
                        $y = if ($p.pos) { $p.pos.y } else { '?' }
                        return "trap - position revealed ($x,$y)"
                    }
                    'random_teleport' { return 'trap - random teleport' }
                    'lose_next_turn'  { return 'trap - lose next turn' }
                    'lose_bullet'     { return 'trap - lost bullet' }
                    'info_blackout'   { return 'trap - info blackout' }
                    default           { return "trap - $eff" }
                }
            }
            'reward_activated' {
                $eff = if ($p -and $p.effect) { $p.effect } else { '' }
                switch ($eff) {
                    'nearest_direction'      { return "reward - nearest player is $($p.direction)" }
                    'all_positions_revealed' { return 'reward - all positions revealed' }
                    'all_bullet_locations'   { return 'reward - all bullet tiles revealed' }
                    default                  { return "reward - $eff" }
                }
            }
            'player_eliminated' {
                $victim = if ($p -and $p.playerName)   { $p.playerName }   else { '?' }
                $by     = if ($p -and $p.byPlayerName) { $p.byPlayerName } else { $null }
                if ($by) { return "$by shot $victim - eliminated" }
                return "$victim - eliminated"
            }
            'shot_fired' {
                $by   = if ($p -and $p.byPlayerName) { $p.byPlayerName } else { '?' }
                $dirn = if ($p -and $p.direction)    { $p.direction }    else { '' }
                return "$by - fired $dirn"
            }
            'map_submitted' {
                $name = if ($p -and $p.playerName) { $p.playerName } else { '?' }
                if ($p -and $p.correct) { return "$name - submitted map - correct! win!" }
                return "$name - submitted map - $($p.wrong) wrong ($($p.submitsLeft) left)"
            }
            'portal_used'   { return 'portal - teleported' }
            'clue_received' { return 'clue received' }
            'turn_skipped'  {
                $name = if ($p -and $p.playerName) { $p.playerName } else { '?' }
                return "$name - turn skipped"
            }
            default { return $ev.kind }
        }
    }

    function Show-GameState($data) {
        $self     = $data.self
        $isMyTurn = ($data.currentTurn -eq $shared.myId)
        $now      = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
        $secsLeft = [Math]::Max(0, [int](($data.turnEndsAt - $now) / 1000))
        $turnNum  = $data.turn

        $turnName = if ($isMyTurn) { 'YOUR TURN' } else {
            $match = $data.others | Where-Object { $_.id -eq $data.currentTurn } | Select-Object -First 1
            if ($match) { "$($match.name)'s turn" } else { "?'s turn" }
        }

        $inv = if ($self.inventory -and $self.inventory.Count -gt 0) {
            ($self.inventory | ForEach-Object { $_.kind }) -join ', '
        } else { '─' }

        $others = if ($data.others -and $data.others.Count -gt 0) {
            ($data.others | ForEach-Object { "$(if ($_.alive) { '●' } else { '✗' }) $($_.name)" }) -join '  '
        } else { '─' }

        # track map size, counts, and paused state
        if ($data.mapStats -and $data.mapStats.mapSize) {
            $shared.gameMapSize = $data.mapStats.mapSize
        }
        if ($data.mapStats -and $data.mapStats.counts) {
            $shared.mapCounts = $data.mapStats.counts
        }
        if ($null -ne $data.paused) {
            $shared.paused = [bool]$data.paused
        }

        $sep = [string]([char]0x2500) * 48

        [Console]::WriteLine('')
        [Console]::WriteLine($sep)
        if ($shared.paused) {
            [Console]::WriteLine("  *** GAME PAUSED — press P to resume ***")
            [Console]::WriteLine($sep)
        }
        [Console]::WriteLine("  Turn $turnNum  |  $turnName  |  $($secsLeft)s left")
        [Console]::WriteLine($sep)
        [Console]::WriteLine("  Explored $($self.visitedCount)/$($self.totalCells)  Items: $inv  Others: $others")

        if ($data.events -and $data.events.Count -gt 0) {
            [Console]::WriteLine($sep)
            foreach ($ev in $data.events) {
                $txt = Format-Event $ev
                [Console]::WriteLine("  > $txt")
                # store in shared action log (keep last 20)
                $shared.actionLog.Add("T$turnNum  $txt")
                while ($shared.actionLog.Count -gt 20) { $shared.actionLog.RemoveAt(0) }
            }
        }

        [Console]::WriteLine($sep)
        if ($isMyTurn -and -not $shared.paused) {
            [Console]::WriteLine('  [W/A/S/D] Move  [F] Shoot  [M] Map  [N] Info  [L] Log  [T] Chat  [P] Pause  [Q] Quit')
        } elseif ($shared.paused) {
            [Console]::WriteLine('  [P] Resume game')
        }
    }

    $buf = [byte[]]::new(65536)
    $sb  = [System.Text.StringBuilder]::new()

    while ($shared.running -and $shared.ws.State -eq 'Open') {
        try {
            $seg = [ArraySegment[byte]]$buf
            $r   = $shared.ws.ReceiveAsync($seg, [System.Threading.CancellationToken]::None).GetAwaiter().GetResult()
            if ($r.MessageType -eq 'Close') { break }
            [void]$sb.Append($enc.GetString($buf, 0, $r.Count))
            if ($r.EndOfMessage) {
                $raw = $sb.ToString()
                [void]$sb.Clear()
                try {
                    $msg = $raw | ConvertFrom-Json
                    switch ($msg.type) {
                        'welcome' {
                            $shared.myId = $msg.data.playerId
                            $code = $msg.data.roomState.roomCode
                            [Console]::WriteLine("Joined room $code")
                        }
                        'lobby_update' {
                            $players = $msg.data.players -join ', '
                            [Console]::WriteLine("Lobby: $players")
                        }
                        'game_start'  { Show-GameState $msg.data }
                        'turn_result' { Show-GameState $msg.data }
                        'error' {
                            $friendly = switch ($msg.data.code) {
                                'INVALID_DIRECTION' { "Wall! Can't move that way." }
                                'NOTHING_TO_PICKUP' { 'Nothing to pick up here.' }
                                'NO_BULLET'         { 'No bullet in inventory.' }
                                'MAP_INCOMPLETE'    { "Map incomplete — $($msg.data.message)" }
                                default             { $msg.data.message }
                            }
                            [Console]::WriteLine("! $friendly")
                        }
                        'game_over' {
                            $winner = $msg.data.winner
                            $reason = switch ($msg.data.winReason) {
                                'last_alive'   { 'last player standing' }
                                'map_complete' { 'mapped the entire board' }
                                default        { $msg.data.winReason }
                            }
                            [Console]::WriteLine('')
                            if ($winner) {
                                [Console]::WriteLine("=== GAME OVER — $winner wins ($reason) ===")
                            } else {
                                [Console]::WriteLine('=== GAME OVER — No winner ===')
                            }
                        }
                        'chat_msg' {
                            [Console]::WriteLine("  [CHAT] $($msg.data.senderName): $($msg.data.text)")
                        }
                        'chat_history' {
                            if ($msg.data.messages.Count -gt 0) {
                                [Console]::WriteLine("  ── chat history ──")
                                foreach ($m in $msg.data.messages) {
                                    [Console]::WriteLine("  [CHAT] $($m.senderName): $($m.text)")
                                }
                                [Console]::WriteLine("  ──────────────────")
                            }
                        }
                        'server_shutdown' { [Console]::WriteLine('Server is restarting...') }
                        'pong'            { }
                        default           { [Console]::WriteLine("[$($msg.type)] $raw") }
                    }
                } catch {
                    [Console]::WriteLine("< $raw")
                }
            }
        } catch { break }
    }
})
$handle = $ps.BeginInvoke()

Write-Host ""
Write-Host "Controls:" -ForegroundColor Yellow
Write-Host "  W/A/S/D or Arrow keys = Move"
Write-Host "  G                     = Start game (host only)"
Write-Host "  M                     = Paint map / submit"
Write-Host "  F then W/A/S/D        = Shoot"
Write-Host "  N                     = Map info (cell counts)"
Write-Host "  L                     = Show last 20 events"
Write-Host "  T                     = Chat"
Write-Host "  P                     = Pause / Resume"
Write-Host "  Q                     = Quit"
Write-Host ""

function Show-MapPainter {
    $size  = $shared.gameMapSize
    $walls = $shared.mapWalls

    function Render-Grid {
        $hdr = "    " + (0..($size-1) -join " ")
        Write-Host $hdr
        $bar = "   +" + ("-" * ($size * 2 - 1)) + "+"
        Write-Host $bar
        for ($y = 0; $y -lt $size; $y++) {
            $row = (0..($size-1) | ForEach-Object {
                if ($walls.Contains("$_,$y")) { [char]0x2588 } else { '.' }
            }) -join " "
            Write-Host "  $y|$row|"
        }
        Write-Host $bar
        Write-Host "  $($walls.Count) walls marked"
        Write-Host "  X Y=toggle  ok=submit  clear=reset  show=redraw  q=cancel"
    }

    Render-Grid
    $done = $false
    while (-not $done) {
        $line = (Read-Host "Map").Trim()
        switch ($line.ToLower()) {
            'q'     { $done = $true }
            'cancel'{ $done = $true }
            'show'  { Render-Grid }
            'clear' { $walls.Clear(); Render-Grid }
            { $_ -in 'ok','submit','sub' } {
                $wallArr = $walls | ForEach-Object {
                    $parts = $_ -split ','
                    "{`"x`":$($parts[0]),`"y`":$($parts[1])}"
                }
                $wallJson = "[" + ($wallArr -join ",") + "]"
                Send-Json (Action "{`"kind`":`"submit_map`",`"walls`":$wallJson}")
                Write-Host "(map submitted)" -ForegroundColor DarkCyan
                $done = $true
            }
            default {
                $parts = $line -split '\s+'
                if ($parts.Count -eq 2) {
                    try {
                        $x = [int]$parts[0]; $y = [int]$parts[1]
                        if ($x -ge 0 -and $x -lt $size -and $y -ge 0 -and $y -lt $size) {
                            $key = "$x,$y"
                            if ($walls.Contains($key)) { [void]$walls.Remove($key); Write-Host "  ($x,$y) cleared" }
                            else { [void]$walls.Add($key); Write-Host "  ($x,$y) marked wall" }
                            Render-Grid
                        } else { Write-Host "  Use 0-$($size-1) for x and y" -ForegroundColor Red }
                    } catch { Write-Host "  Type: X Y  or  ok  or  q" -ForegroundColor Red }
                } else { Write-Host "  Type: X Y  or  ok  or  clear  or  q" -ForegroundColor Red }
            }
        }
    }
}

function Show-ActionLog {
    $sep = [string]([char]0x2500) * 48
    Write-Host ""
    Write-Host $sep
    Write-Host ("  {0,-8}  {1}" -f "Turn","Event")
    Write-Host $sep
    if ($shared.actionLog.Count -eq 0) {
        Write-Host "  (no events yet)"
    } else {
        foreach ($e in $shared.actionLog) { Write-Host "  $e" }
    }
    Write-Host $sep
}

function Show-MapInfo {
    $sep = [string]([char]0x2500) * 48
    $counts = $shared.mapCounts
    if (-not $counts) { Write-Host "  (map info not available yet)"; return }
    Write-Host ""
    Write-Host $sep
    Write-Host "  Map info ($($shared.gameMapSize)x$($shared.gameMapSize))"
    Write-Host $sep
    Write-Host ("  {0,-10} {1}" -f "Type","Count")
    Write-Host $sep
    @("blank","wall","bullet","reward","trap","portal") | ForEach-Object {
        $val = $counts.$_
        if ($null -eq $val) { $val = 0 }
        Write-Host ("  {0,-10} {1}" -f $_,$val)
    }
    Write-Host $sep
}

$shootPending = $false
while ($shared.ws.State -eq 'Open') {
    $k = [Console]::ReadKey($true)

    if ($shootPending) {
        $shootPending = $false
        $dir = switch ($k.Key) {
            'W' { 'N' } 'UpArrow'    { 'N' }
            'S' { 'S' } 'DownArrow'  { 'S' }
            'A' { 'W' } 'LeftArrow'  { 'W' }
            'D' { 'E' } 'RightArrow' { 'E' }
            default { $null }
        }
        if ($dir) {
            Write-Host "(shoot $dir)" -ForegroundColor DarkGray
            Send-Json (Action "{`"kind`":`"shoot`",`"direction`":`"$dir`"}")
        } else {
            Write-Host "(shoot cancelled)" -ForegroundColor DarkGray
        }
        continue
    }

    $json  = $null
    $label = $null
    switch ($k.Key) {
        'W'          { $json = Action '{"kind":"move","direction":"N"}'; $label = '(move N)' }
        'UpArrow'    { $json = Action '{"kind":"move","direction":"N"}'; $label = '(move N)' }
        'S'          { $json = Action '{"kind":"move","direction":"S"}'; $label = '(move S)' }
        'DownArrow'  { $json = Action '{"kind":"move","direction":"S"}'; $label = '(move S)' }
        'A'          { $json = Action '{"kind":"move","direction":"W"}'; $label = '(move W)' }
        'LeftArrow'  { $json = Action '{"kind":"move","direction":"W"}'; $label = '(move W)' }
        'D'          { $json = Action '{"kind":"move","direction":"E"}'; $label = '(move E)' }
        'RightArrow' { $json = Action '{"kind":"move","direction":"E"}'; $label = '(move E)' }
        'G'          { $json = Action "{`"kind`":`"start_game`",`"mapSize`":$($shared.mapSize),`"turnSeconds`":$($shared.turnSecs)}"; $label = '(starting game…)' }
        'M'          { Show-MapPainter }
        'L'          { Show-ActionLog }
        'N'          { Show-MapInfo }
        'P'          {
                       if ($shared.paused) {
                           $json = Action '{"kind":"resume"}'; $label = '(resuming…)'
                       } else {
                           $json = Action '{"kind":"pause"}'; $label = '(pausing…)'
                       }
                     }
        'F'          { $shootPending = $true
                       Write-Host "Shoot direction: W/A/S/D" -ForegroundColor DarkCyan }
        'T'          {
                       $chatText = Read-Host "Chat"
                       if ($chatText.Trim()) {
                           $json = ChatEnvelope $chatText.Trim()
                       }
                     }
        'Q'          { $shared.running = $false
                       [void]$ws.CloseOutputAsync('NormalClosure','bye',$ct).GetAwaiter().GetResult()
                       break }
    }
    if ($label) { Write-Host $label -ForegroundColor DarkGray }
    if ($json)  { Send-Json $json }
}

$ps.EndInvoke($handle) | Out-Null
$rs.Close()
Write-Host "Disconnected."
