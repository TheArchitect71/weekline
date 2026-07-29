[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$CaddyExecutable,

    [string]$OutputDirectory = ""
)

$ErrorActionPreference = "Stop"
$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
if (-not $OutputDirectory) {
    $OutputDirectory = Join-Path $RepositoryRoot "dist\windows"
}

if (-not (Test-Path $CaddyExecutable -PathType Leaf)) {
    throw "Caddy executable not found: $CaddyExecutable"
}

$ExpectedDenoVersion = (Get-Content (Join-Path $RepositoryRoot ".deno-version") -Raw).Trim()
$denoVersionText = ((& deno --version | Select-Object -First 1) -replace '^deno\s+', '').Trim()
$denoVersion = [Version]$denoVersionText
if ($denoVersion -ne [Version]$ExpectedDenoVersion) {
    throw "Deno $ExpectedDenoVersion is required for a reproducible manager build. Found $denoVersionText."
}

New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $OutputDirectory "web") -Force | Out-Null

Push-Location (Join-Path $RepositoryRoot "web")
try {
    pnpm install --frozen-lockfile
    pnpm build
}
finally {
    Pop-Location
}

Push-Location (Join-Path $RepositoryRoot "api")
try {
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -trimpath -ldflags "-s -w" -o (Join-Path $OutputDirectory "weekline-host.exe") ./cmd/server
}
finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Pop-Location
}

deno desktop `
    --backend cef `
    --target x86_64-pc-windows-msvc `
    --allow-env `
    --allow-read `
    --allow-net `
    --allow-sys=hostname `
    --output (Join-Path $OutputDirectory "WeeklineManager.msi") `
    (Join-Path $RepositoryRoot "desktop\main.ts")

Copy-Item $CaddyExecutable (Join-Path $OutputDirectory "caddy.exe") -Force
Copy-Item (Join-Path $RepositoryRoot "deploy\Caddyfile") (Join-Path $OutputDirectory "Caddyfile") -Force
Copy-Item (Join-Path $RepositoryRoot "web\dist\web\browser\*") (Join-Path $OutputDirectory "web") -Recurse -Force
Copy-Item (Join-Path $PSScriptRoot "install-office-host.ps1") $OutputDirectory -Force
Copy-Item (Join-Path $PSScriptRoot "install-manager.ps1") $OutputDirectory -Force
Copy-Item (Join-Path $PSScriptRoot "verify-office-deployment.ps1") $OutputDirectory -Force
Copy-Item (Join-Path $RepositoryRoot "VERSION") $OutputDirectory -Force

Write-Host "Weekline Windows release prepared at $OutputDirectory"
