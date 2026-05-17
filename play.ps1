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

$ws  = [System.Net.WebSockets.ClientWebSocket]::new()
$ct  = [System.Threading.CancellationToken]::None
$enc = [System.Text.Encoding]::UTF8

Write-Host "Connecting..." -ForegroundColor DarkGray
$ws.ConnectAsync([Uri]$Url, $ct).GetAwaiter().GetResult()
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
$shared = [hashtable]::Synchronized(@{ ws = $ws; running = $true })
$rs = [System.Management.Automation.Runspaces.RunspaceFactory]::CreateRunspace()
$rs.Open()
$rs.SessionStateProxy.SetVariable('shared', $shared)
$rs.SessionStateProxy.SetVariable('enc', $enc)
$ps = [powershell]::Create()
$ps.Runspace = $rs
[void]$ps.AddScript({
    $buf = [byte[]]::new(16384)
    $sb  = [System.Text.StringBuilder]::new()
    while ($shared.running -and $shared.ws.State -eq 'Open') {
        try {
            $seg = [ArraySegment[byte]]$buf
            $r   = $shared.ws.ReceiveAsync($seg, [System.Threading.CancellationToken]::None).GetAwaiter().GetResult()
            if ($r.MessageType -eq 'Close') { break }
            [void]$sb.Append($enc.GetString($buf, 0, $r.Count))
            if ($r.EndOfMessage) {
                [Console]::WriteLine('< ' + $sb.ToString())
                [void]$sb.Clear()
            }
        } catch { break }
    }
})
$handle = $ps.BeginInvoke()

# Join room
$joinJson = '{"type":"join","ts":' + (Now) + ',"data":{"roomCode":"' + $Room + '","playerName":"' + $Name + '"}}'
Send-Json $joinJson

Write-Host ""
Write-Host "Controls:" -ForegroundColor Yellow
Write-Host "  W / Arrow-Up    = Move North"
Write-Host "  S / Arrow-Down  = Move South"
Write-Host "  A / Arrow-Left  = Move West"
Write-Host "  D / Arrow-Right = Move East"
Write-Host "  G               = Start game (host only)"
Write-Host "  P               = Pick up item"
Write-Host "  Q               = Quit"
Write-Host ""

while ($shared.ws.State -eq 'Open') {
    $k = [Console]::ReadKey($true)
    $json = switch ($k.Key) {
        'W'        { Action '{"kind":"move","direction":"N"}' }
        'UpArrow'  { Action '{"kind":"move","direction":"N"}' }
        'S'        { Action '{"kind":"move","direction":"S"}' }
        'DownArrow'{ Action '{"kind":"move","direction":"S"}' }
        'A'        { Action '{"kind":"move","direction":"W"}' }
        'LeftArrow'{ Action '{"kind":"move","direction":"W"}' }
        'D'        { Action '{"kind":"move","direction":"E"}' }
        'RightArrow'{Action '{"kind":"move","direction":"E"}' }
        'G'        { Action '{"kind":"start_game"}' }
        'P'        { Action '{"kind":"pickup"}' }
        'Q'        { $shared.running = $false
                     $ws.CloseAsync('NormalClosure','bye',$ct).GetAwaiter().GetResult()
                     break }
        default    { $null }
    }
    if ($json) { Send-Json $json }
}

$ps.EndInvoke($handle) | Out-Null
$rs.Close()
Write-Host "Disconnected."
