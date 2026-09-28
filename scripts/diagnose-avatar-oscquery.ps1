# Run from the repository root with:
# powershell -NoProfile -ExecutionPolicy Bypass -File scripts/diagnose-avatar-oscquery.ps1
param(
    [ValidateRange(0, 65535)]
    [int]$Port = 0
)

$ErrorActionPreference = 'Stop'

if ($Port -eq 0) {
    $vrchat = Get-Process -Name VRChat -ErrorAction Stop | Select-Object -First 1
    $listeners = netstat -ano -p tcp
    $ports = foreach ($line in $listeners) {
        if ($line -match ('^\s*TCP\s+127\.0\.0\.1:(\d+)\s+\S+\s+LISTENING\s+' + $vrchat.Id + '\s*$')) {
            [int]$Matches[1]
        }
    }
    foreach ($candidate in ($ports | Sort-Object -Unique)) {
        try {
            $info = Invoke-RestMethod -TimeoutSec 2 -Uri "http://127.0.0.1:$candidate/?HOST_INFO"
            if ($info.NAME -like '*VRChat*') {
                $Port = $candidate
                break
            }
        } catch {
            continue
        }
    }
    if ($Port -eq 0) {
        throw 'VRChat is running, but no local OSCQuery HTTP listener was found. Supply -Port if its port is known.'
    }
}

$baseURL = "http://127.0.0.1:$Port"
$hostInfo = Invoke-RestMethod -TimeoutSec 3 -Uri "$baseURL/?HOST_INFO"
$avatar = Invoke-RestMethod -TimeoutSec 3 -Uri "$baseURL/avatar"
$change = $avatar.CONTENTS.change
$avatarID = if ($null -ne $change -and $change.VALUE.Count -eq 1) { [string]$change.VALUE[0] } else { '' }
$readable = $null -ne $change -and ($null -eq $change.ACCESS -or ([int]$change.ACCESS -band 1) -ne 0)
$queryUsable = $change.TYPE -eq 's' -and $readable -and $avatarID.Length -gt 0
$localTestAvatar = $avatarID.StartsWith('local:sdk_')
$filename = if ($localTestAvatar) { $avatarID.Substring('local:sdk_'.Length) } else { $avatarID }
$unsafe = $avatarID.Length -eq 0 -or $avatarID.Length -gt 256 -or $filename -eq '' -or $filename -eq '.' -or $filename -eq '..'
foreach ($char in @('<', '>', ':', '"', '/', '\', '|', '?', '*', '[', ']', [char]0)) {
    if ($filename.Contains([string]$char)) {
        $unsafe = $true
    }
}
$settingsPath = Join-Path $env:APPDATA 'vrcft-go/config.json'
$fallbackPath = ''
if (Test-Path -LiteralPath $settingsPath) {
    $settings = Get-Content -LiteralPath $settingsPath -Raw | ConvertFrom-Json
    $fallbackPath = [string]$settings.avatar.fallbackPath
}
$fallbackReady = $fallbackPath -ne '' -and (Test-Path -LiteralPath $fallbackPath -PathType Leaf)

[pscustomobject]@{
    OSCQueryURL = $baseURL
    ServiceName = $hostInfo.NAME
    AvatarNode = $avatar.FULL_PATH
    ChangeType = $change.TYPE
    ChangeAccess = $change.ACCESS
    AvatarID = $avatarID
    StartupQueryUsable = $queryUsable
    PlannerAcceptsID = -not $unsafe
    LocalTestAvatar = $localTestAvatar
    FallbackConfigured = $fallbackPath -ne ''
    FallbackFileExists = $fallbackReady
    PlanCanUseFallback = $localTestAvatar -and -not $unsafe -and $fallbackReady
} | Format-List

$logPath = Join-Path $env:APPDATA 'vrcft-go/logs/application.jsonl'
if (Test-Path -LiteralPath $logPath) {
    $recent = Get-Content -LiteralPath $logPath -Tail 200 |
        Where-Object { $_ -match '"stage":"avatar_plan"' } |
        Select-Object -Last 3
    if ($recent) {
        Write-Host 'Recent application avatar-plan log entries:'
        $recent
    }
}

if (-not $queryUsable) {
    exit 1
}
