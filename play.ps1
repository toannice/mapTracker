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
$shared = [hashtable]::Synchronized(@{ ws = $ws; running = $true; myId = ''; playerName = $Name; mapSize = $MapSize; turnSecs = $TurnSecs })
$rs = [System.Management.Automation.Runspaces.RunspaceFactory]::CreateRunspace()
$rs.Open()
$rs.SessionStateProxy.SetVariable('shared', $shared)
$rs.SessionStateProxy.SetVariable('enc', $enc)
$ps = [powershell]::Create()
$ps.Runspace = $rs
[void]$ps.AddScript({
    function Format-Event($ev) {
        switch ($ev.kind) {
            'trap_triggered'    { return 'You triggered a trap!' }
            'reward_activated'  { return 'You found a reward!' }
            'player_eliminated' {
                if ($ev.payload -ne $null -and $ev.payload.playerName -ne $null -and $ev.payload.byPlayerName -ne $null) {
                    return "$($ev.payload.playerName) was eliminated by $($ev.payload.byPlayerName)"
                } elseif ($ev.payload -ne $null -and $ev.payload.playerName -ne $null) {
                    return "$($ev.payload.playerName) was eliminated!"
                }
                return 'A player was eliminated!'
            }
            'shot_fired' {
                if ($ev.payload -ne $null -and $ev.payload.direction -ne $null) {
                    return "Shot fired $($ev.payload.direction)!"
                }
                return 'Shot fired!'
            }
            'map_submitted' {
                if ($ev.payload -ne $null -and $ev.payload.playerName -ne $null) {
                    return "$($ev.payload.playerName) submitted their map!"
                }
                return 'Map submitted!'
            }
            'portal_used'   { return 'You teleported!' }
            'clue_received' { return 'Clue received!' }
            'turn_skipped'  { return 'Turn skipped (timed out)' }
            default         { return $ev.kind }
        }
    }

    function Show-GameState($data) {
        $self     = $data.self
        $isMyTurn = ($data.currentTurn -eq $shared.myId)
        $now      = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
        $secsLeft = [Math]::Max(0, [int](($data.turnEndsAt - $now) / 1000))

        $turnName = if ($isMyTurn) {
            $shared.playerName
        } else {
            $match = $data.others | Where-Object { $_.id -eq $data.currentTurn } | Select-Object -First 1
            if ($match) { $match.name } else { '?' }
        }

        $inv = if ($self.inventory -and $self.inventory.Count -gt 0) {
            ($self.inventory | ForEach-Object { $_.kind }) -join ', '
        } else { 'empty' }

        $others = if ($data.others -and $data.others.Count -gt 0) {
            ($data.others | ForEach-Object { "$(if ($_.alive) { [char]0x2713 } else { [char]0x2717 }) $($_.name)" }) -join '  '
        } else { 'none' }

        $sep = [string]([char]0x2500) * 44

        [Console]::WriteLine('')
        [Console]::WriteLine($sep)
        $turnStatus = if ($isMyTurn) { 'YOUR TURN' } else { "$turnName's turn" }
        [Console]::WriteLine("  Turn $($data.turn)  |  $turnStatus  |  $($secsLeft)s left")
        [Console]::WriteLine($sep)
        [Console]::WriteLine("  Explored: $($self.visitedCount)/$($self.totalCells)")
        [Console]::WriteLine("  Items: $inv")
        [Console]::WriteLine("  Others: $others")

        if ($data.events -and $data.events.Count -gt 0) {
            [Console]::WriteLine($sep)
            foreach ($ev in $data.events) {
                [Console]::WriteLine("  > $(Format-Event $ev)")
            }
        }

        [Console]::WriteLine($sep)
        if ($isMyTurn) {
            [Console]::WriteLine('  [W/A/S/D] Move   [M] Submit map   [F] Shoot   [Q] Quit')
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
                            $t = [DateTimeOffset]::FromUnixTimeMilliseconds($msg.data.ts).LocalDateTime.ToString("HH:mm")
                            [Console]::WriteLine("[$t] $($msg.data.senderName): $($msg.data.text)")
                        }
                        'chat_history' {
                            if ($msg.data.messages.Count -gt 0) {
                                [Console]::WriteLine("── chat history ──")
                                foreach ($m in $msg.data.messages) {
                                    $t = [DateTimeOffset]::FromUnixTimeMilliseconds($m.ts).LocalDateTime.ToString("HH:mm")
                                    [Console]::WriteLine("[$t] $($m.senderName): $($m.text)")
                                }
                                [Console]::WriteLine("──────────────────")
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
Write-Host "  W/A/S/D or Arrow keys = Move  (bullets auto-picked up on move)"
Write-Host "  G                     = Start game (host only)"
Write-Host "  M                     = Submit map (win condition)"
Write-Host "  F then W/A/S/D        = Shoot in direction"
Write-Host "  T                     = Type a chat message"
Write-Host "  Q                     = Quit"
Write-Host ""

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
        'M'          { $json = Action '{"kind":"submit_map"}'; $label = '(submitting map…)' }
        'F'          { $shootPending = $true
                       Write-Host "Shoot direction: W/A/S/D" -ForegroundColor DarkCyan }
        'T'          {
                       $chatText = Read-Host "Chat"
                       if ($chatText.Trim()) {
                           $json = ChatEnvelope $chatText.Trim()
                           $label = $null  # no extra label, the broadcast echo is enough
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
