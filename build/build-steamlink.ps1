[CmdletBinding()]
param(
    [string]$PluginRoot
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
if ([string]::IsNullOrWhiteSpace($PluginRoot)) {
    $PluginRoot = Join-Path $repoPath 'build/bin/plugins'
}
$pluginRootPath = [System.IO.Path]::GetFullPath($PluginRoot)
$pluginDir = [System.IO.Path]::GetFullPath((Join-Path $pluginRootPath 'steamlink'))
$manifestPath = Join-Path $repoPath 'plugins/steamlink/manifest.json'
$executablePath = Join-Path $pluginDir 'steamlink-plugin.exe'

$env:GOCACHE = 'F:\dev\vrcft-go\.go-gocache'
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
New-Item -ItemType Directory -Force -Path $pluginDir | Out-Null

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

    $buildArgs = @('build', '-o', $executablePath, './cmd/steamlink-plugin')
    & go @buildArgs
    if ($LASTEXITCODE -ne 0) {
        throw 'Steam Link plugin build failed'
    }
    Copy-Item -LiteralPath $manifestPath -Destination (Join-Path $pluginDir 'manifest.json') -Force
} finally {
    Pop-Location
}
