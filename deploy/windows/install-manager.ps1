[CmdletBinding()]
param(
    [string]$SourceDirectory = $PSScriptRoot,
    [string]$ConfigDirectory = "$env:APPDATA\Weekline",
    [switch]$SkipMsiInstall
)

$ErrorActionPreference = "Stop"
$MsiPath = Join-Path $SourceDirectory "WeeklineManager.msi"
$SourceConfigPath = Join-Path $SourceDirectory "manager.json"

if (-not (Test-Path $SourceConfigPath -PathType Leaf)) {
    throw "Manager configuration not found: $SourceConfigPath"
}
$config = Get-Content $SourceConfigPath -Raw | ConvertFrom-Json
$serverUrl = [string]$config.serverUrl
$hostControlToken = [string]$config.hostControlToken
if (-not $serverUrl.StartsWith("https://")) {
    throw "manager.json must contain an HTTPS serverUrl."
}
if ($hostControlToken.Length -lt 32) {
    throw "manager.json must contain a hostControlToken with at least 32 characters."
}

if (-not $SkipMsiInstall) {
    if (-not (Test-Path $MsiPath -PathType Leaf)) {
        throw "Manager MSI not found: $MsiPath"
    }
    $install = Start-Process `
        -FilePath "msiexec.exe" `
        -ArgumentList @("/i", "`"$MsiPath`"") `
        -Verb RunAs `
        -Wait `
        -PassThru
    if ($install.ExitCode -notin @(0, 3010)) {
        throw "WeeklineManager.msi failed with Windows Installer exit code $($install.ExitCode)."
    }
}

New-Item -ItemType Directory -Path $ConfigDirectory -Force | Out-Null
$TargetConfigPath = Join-Path $ConfigDirectory "manager.json"
$Utf8NoBom = New-Object Text.UTF8Encoding($false)
$normalizedConfig = @{
    serverUrl = $serverUrl.TrimEnd("/")
    hostControlToken = $hostControlToken
    appVersion = if ($config.appVersion) { [string]$config.appVersion } else { "0.1.0" }
} | ConvertTo-Json
[IO.File]::WriteAllText($TargetConfigPath, $normalizedConfig, $Utf8NoBom)

$currentUser = [Security.Principal.WindowsIdentity]::GetCurrent().Name
& icacls.exe $TargetConfigPath /inheritance:r /grant:r "${currentUser}:(F)" "*S-1-5-18:(F)" "*S-1-5-32-544:(F)" | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw "Unable to restrict access to $TargetConfigPath."
}

Write-Host "Weekline Manager is configured for $currentUser."
Write-Host "Configuration: $TargetConfigPath"
if ($install -and $install.ExitCode -eq 3010) {
    Write-Host "Windows Installer requested a restart before the application is opened."
}
