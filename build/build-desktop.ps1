[CmdletBinding()]
param(
    [switch]$NSIS
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Add-GitSafeDirectory {
    param([Parameter(Mandatory = $true)][string]$Directory)

    $count = 0
    if (-not [string]::IsNullOrWhiteSpace($env:GIT_CONFIG_COUNT)) {
        if (-not [int]::TryParse($env:GIT_CONFIG_COUNT, [ref]$count) -or $count -lt 0) {
            throw 'GIT_CONFIG_COUNT must be a non-negative integer'
        }
    }
    Set-Item -Path ("Env:GIT_CONFIG_KEY_{0}" -f $count) -Value 'safe.directory'
    Set-Item -Path ("Env:GIT_CONFIG_VALUE_{0}" -f $count) -Value $Directory
    $env:GIT_CONFIG_COUNT = ($count + 1).ToString()
}

$repoPath = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$pluginRoot = [System.IO.Path]::GetFullPath((Join-Path $repoPath 'build/bin/plugins'))
$desktopExecutable = Join-Path $repoPath 'build/bin/vrcft-go2.exe'
$pluginScript = Join-Path $repoPath 'build/build-steamlink.ps1'

$env:GOCACHE = 'F:\dev\vrcft-go\.go-gocache'
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null

Push-Location $repoPath
try {
    Add-GitSafeDirectory -Directory $repoPath
    $commonGitDir = (& git rev-parse --git-common-dir).Trim()
    if ($LASTEXITCODE -ne 0) {
        throw 'Unable to resolve the Git common directory'
    }
    if (-not [System.IO.Path]::IsPathRooted($commonGitDir)) {
        $commonGitDir = Join-Path $repoPath $commonGitDir
    }
    Add-GitSafeDirectory -Directory ([System.IO.Path]::GetFullPath((Join-Path $commonGitDir '..')))

    $buildArgs = @('build', '-o', 'vrcft-go2.exe', '-nopackage')
    & wails @buildArgs
    if ($LASTEXITCODE -ne 0) {
        throw 'Desktop build failed'
    }

    & $pluginScript -PluginRoot $pluginRoot
    if ($LASTEXITCODE -ne 0) {
        throw 'Steam Link plugin staging failed'
    }

    $requiredPaths = @(
        $desktopExecutable,
        (Join-Path $pluginRoot 'steamlink/manifest.json'),
        (Join-Path $pluginRoot 'steamlink/steamlink-plugin.exe')
    )
    foreach ($path in $requiredPaths) {
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "Desktop build did not stage required file: $path"
        }
    }

    if ($NSIS) {
        # This invocation does not use -clean, so the plugin staged above remains
        # available to the existing NSIS script when wails.files expands.
        $nsisArgs = @('build', '-o', 'vrcft-go2.exe', '-nsis', '-s', '-m')
        & wails @nsisArgs
        if ($LASTEXITCODE -ne 0) {
            throw 'NSIS installer build failed'
        }
        foreach ($path in $requiredPaths) {
            if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
                throw "NSIS packaging removed staged required file: $path"
            }
        }
    }
} finally {
    Pop-Location
}
