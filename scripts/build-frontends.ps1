<#
.SYNOPSIS
    Builds the static frontend(s) and copies their output into the Go
    binary's embed directories (internal/webreact/dist, internal/webqwik/dist).

.DESCRIPTION
    Each frontend package (web-react/, web-qwik/) is a standalone npm
    project built independently of `go build`; its dist/ output is
    gitignored, and internal/web{react,qwik}/dist only ships a
    placeholder .gitkeep so a fresh checkout builds without Node. Run this
    script (or the equivalent npm commands by hand) before starting the
    server with WEB_FRONTEND=react or WEB_FRONTEND=qwik.

.PARAMETER Frontend
    Which frontend(s) to build: react, qwik, or all (default).
#>
param(
    [ValidateSet('react', 'qwik', 'all')]
    [string]$Frontend = 'all'
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot

function Build-Frontend([string]$name) {
    $srcDir = Join-Path $repoRoot "web-$name"
    $embedDist = Join-Path $repoRoot "internal\web$name\dist"

    if (-not (Test-Path $srcDir)) {
        Write-Host "Skipping $name: $srcDir does not exist yet."
        return
    }

    Write-Host "Building web-$name..."
    Push-Location $srcDir
    try {
        npm ci
        npm run build
    } finally {
        Pop-Location
    }

    $builtDist = Join-Path $srcDir 'dist'
    if (-not (Test-Path $builtDist)) {
        throw "$srcDir\dist was not produced by 'npm run build'"
    }

    Get-ChildItem $embedDist -Force | Where-Object { $_.Name -ne '.gitkeep' } |
        Remove-Item -Recurse -Force
    Copy-Item (Join-Path $builtDist '*') $embedDist -Recurse -Force

    Write-Host "Copied web-$name/dist -> internal/web$name/dist"
}

$targets = if ($Frontend -eq 'all') { 'react', 'qwik' } else { @($Frontend) }
foreach ($t in $targets) {
    Build-Frontend $t
}
