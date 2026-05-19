# Blind Map Survival — terminal client
# Usage: .\play.ps1  (or  .\play.ps1 -Url wss://... -Name Bob -Room ABC123)
param(
    [string]$Url  = "wss://maptracker-c68n.onrender.com/ws",
    [string]$Name = "",
    [string]$Room = ""
)

if (!$Name) { $Name = Read-Host "Your name" }
if (!$Room) {
    $c = Read-Host "C=Create room   J=Join room"
    if ($c -ieq 'C') {
        $Room = -join ((65..90 + 48..57) | Get-Random -Count 6 | ForEach-Object { [char]$_ })
        Write-Host "Room code: $Room  (share this with the other player)" -ForegroundColor Cyan
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
        $ws.ConnectAsync([Uri]$wsUrl, $ct).GetAwaiter().GetResult()
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
    $ws.SendAsync([ArraySegment[byte]]$b, 'Text', $true, $ct).GetAwaiter().GetResult()
}
function Now { [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds() }
function Action([string]$dataJson) {
    '{"type":"action","ts":' + (Now) + ',"data":' + $dataJson + '}'
}

# Background receive loop
$shared = [hashtable]::Synchronized(@{ ws = $ws; running = $true; myId = ''; playerName = $Name })
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
        [Console]::WriteLine("  Pos: ($($self.pos.x),$($self.pos.y))   Explored: $($self.visitedCount)/$($self.totalCells)")
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
            [Console]::WriteLine('  [W/A/S/D] Move   [P] Pick up   [M] Submit map   [F] Shoot   [Q] Quit')
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
Write-Host "  P                     = Pick up item"
Write-Host "  M                     = Submit map (win condition)"
Write-Host "  F then W/A/S/D        = Shoot in direction"
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
            Send-Json (Action "{`"kind`":`"shoot`",`"direction`":`"$dir`"}")
        } else {
            Write-Host "(shoot cancelled)" -ForegroundColor DarkGray
        }
        continue
    }

    $json = switch ($k.Key) {
        'W'         { Action '{"kind":"move","direction":"N"}' }
        'UpArrow'   { Action '{"kind":"move","direction":"N"}' }
        'S'         { Action '{"kind":"move","direction":"S"}' }
        'DownArrow' { Action '{"kind":"move","direction":"S"}' }
        'A'         { Action '{"kind":"move","direction":"W"}' }
        'LeftArrow' { Action '{"kind":"move","direction":"W"}' }
        'D'         { Action '{"kind":"move","direction":"E"}' }
        'RightArrow'{ Action '{"kind":"move","direction":"E"}' }
        'G'         { Action '{"kind":"start_game"}' }
        'P'         { Action '{"kind":"pickup"}' }
        'M'         { Action '{"kind":"submit_map"}' }
        'F'         { $shootPending = $true
                      Write-Host "Shoot direction: W/A/S/D" -ForegroundColor DarkCyan
                      $null }
        'Q'         { $shared.running = $false
                      $ws.CloseOutputAsync('NormalClosure','bye',$ct).GetAwaiter().GetResult()
                      break }
        default     { $null }
    }
    if ($json) { Send-Json $json }
}

$ps.EndInvoke($handle) | Out-Null
$rs.Close()
Write-Host "Disconnected."
