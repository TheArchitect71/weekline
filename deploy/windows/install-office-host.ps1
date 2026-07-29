[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Domain,

    [Parameter(Mandatory = $true)]
    [string]$DatabaseUrl,

    [string]$SourceDirectory = $PSScriptRoot,
    [string]$InstallRoot = "$env:ProgramData\Weekline",
    [string]$HostControlToken = "",
    [string]$ManagerEmail = "",
    [string]$ManagerName = "",
    [SecureString]$ManagerPassword
)

$ErrorActionPreference = "Stop"
$ServiceName = "WeeklineHost"

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run this installer from an elevated PowerShell window."
}

function Assert-SingleLine([string]$Name, [string]$Value) {
    if (-not $Value -or $Value.Contains("`r") -or $Value.Contains("`n")) {
        throw "$Name must be a non-empty single-line value."
    }
}

Assert-SingleLine "Domain" $Domain
Assert-SingleLine "DatabaseUrl" $DatabaseUrl
if ([Uri]::CheckHostName($Domain) -ne [UriHostNameType]::Dns) {
    throw "Domain must be a DNS hostname such as schedule.example.com, without a scheme, port, path, or Caddyfile syntax."
}
if (-not $ManagerEmail) {
    $ManagerEmail = Read-Host "Initial Weekline manager email"
}
if (-not $ManagerName) {
    $ManagerName = Read-Host "Initial Weekline manager display name"
}
if (-not $ManagerPassword) {
    $ManagerPassword = Read-Host "Initial Weekline manager password (12-72 bytes)" -AsSecureString
}
Assert-SingleLine "ManagerEmail" $ManagerEmail
Assert-SingleLine "ManagerName" $ManagerName

$required = @("weekline-host.exe", "caddy.exe", "Caddyfile", "WeeklineManager.msi", "install-manager.ps1", "verify-office-deployment.ps1", "VERSION")
foreach ($name in $required) {
    if (-not (Test-Path (Join-Path $SourceDirectory $name) -PathType Leaf)) {
        throw "Release artifact is missing: $name"
    }
}
if (-not (Test-Path (Join-Path $SourceDirectory "web") -PathType Container)) {
    throw "Release artifact is missing the compiled web directory."
}

if (-not $HostControlToken) {
    $bytes = New-Object byte[] 32
    $generator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $generator.GetBytes($bytes) } finally { $generator.Dispose() }
    $HostControlToken = -join ($bytes | ForEach-Object { $_.ToString("x2") })
}
if ($HostControlToken.Length -lt 32) {
    throw "HostControlToken must contain at least 32 characters."
}

$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existing -and $existing.Status -ne "Stopped") {
    Stop-Service -Name $ServiceName -Force
    $existing.WaitForStatus("Stopped", [TimeSpan]::FromSeconds(30))
}

$BinDirectory = Join-Path $InstallRoot "bin"
$WebDirectory = Join-Path $InstallRoot "web"
$ManagerDirectory = Join-Path $InstallRoot "manager"
New-Item -ItemType Directory -Path $BinDirectory -Force | Out-Null
New-Item -ItemType Directory -Path $WebDirectory -Force | Out-Null
New-Item -ItemType Directory -Path $ManagerDirectory -Force | Out-Null

Copy-Item (Join-Path $SourceDirectory "weekline-host.exe") (Join-Path $BinDirectory "weekline-host.exe") -Force
Copy-Item (Join-Path $SourceDirectory "caddy.exe") (Join-Path $BinDirectory "caddy.exe") -Force
Copy-Item (Join-Path $SourceDirectory "WeeklineManager.msi") (Join-Path $ManagerDirectory "WeeklineManager.msi") -Force
Copy-Item (Join-Path $SourceDirectory "install-manager.ps1") (Join-Path $ManagerDirectory "install-manager.ps1") -Force
Copy-Item (Join-Path $SourceDirectory "Caddyfile") (Join-Path $InstallRoot "Caddyfile") -Force
Copy-Item (Join-Path $SourceDirectory "verify-office-deployment.ps1") (Join-Path $InstallRoot "verify-office-deployment.ps1") -Force
Copy-Item (Join-Path $SourceDirectory "web\*") $WebDirectory -Recurse -Force

$ConfigPath = Join-Path $InstallRoot "weekline.env"
$CaddyPath = Join-Path $BinDirectory "caddy.exe"
$CaddyConfig = Join-Path $InstallRoot "Caddyfile"
$Utf8NoBom = New-Object Text.UTF8Encoding($false)
$ConfigContent = @"
WEEKLINE_DATABASE_URL=$DatabaseUrl
WEEKLINE_ADDRESS=127.0.0.1:8080
WEEKLINE_COOKIE_SECURE=true
WEEKLINE_SEED_DEMO=false
WEEKLINE_HOST_CONTROL_TOKEN=$HostControlToken
WEEKLINE_HOST_LEASE_TTL=45s
WEEKLINE_REQUIRE_MANAGER_LEASE=true
WEEKLINE_ENABLE_ATTENDANCE=false
WEEKLINE_CADDY_EXECUTABLE=$CaddyPath
WEEKLINE_CADDY_CONFIG=$CaddyConfig
WEEKLINE_DOMAIN=$Domain
WEEKLINE_WEB_ROOT=$WebDirectory
"@
[IO.File]::WriteAllText($ConfigPath, $ConfigContent, $Utf8NoBom)

$ManagerConfigPath = Join-Path $ManagerDirectory "manager.json"
$AppVersion = (Get-Content (Join-Path $SourceDirectory "VERSION") -Raw).Trim()
Assert-SingleLine "AppVersion" $AppVersion
$ManagerConfig = @{
    serverUrl = "https://$Domain"
    hostControlToken = $HostControlToken
    appVersion = $AppVersion
} | ConvertTo-Json
[IO.File]::WriteAllText($ManagerConfigPath, $ManagerConfig, $Utf8NoBom)

& icacls.exe $ConfigPath /inheritance:r /grant:r "SYSTEM:(F)" "BUILTIN\Administrators:(F)" | Out-Null
& icacls.exe $ManagerConfigPath /inheritance:r /grant:r "SYSTEM:(F)" "BUILTIN\Administrators:(F)" | Out-Null

$HostExecutable = Join-Path $BinDirectory "weekline-host.exe"
$passwordPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($ManagerPassword)
try {
    $env:WEEKLINE_BOOTSTRAP_MANAGER_EMAIL = $ManagerEmail
    $env:WEEKLINE_BOOTSTRAP_MANAGER_NAME = $ManagerName
    $env:WEEKLINE_BOOTSTRAP_MANAGER_PASSWORD = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPointer)
    & $HostExecutable --config $ConfigPath --bootstrap-manager
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to create or verify the initial Weekline manager account."
    }
}
finally {
    Remove-Item Env:WEEKLINE_BOOTSTRAP_MANAGER_EMAIL -ErrorAction SilentlyContinue
    Remove-Item Env:WEEKLINE_BOOTSTRAP_MANAGER_NAME -ErrorAction SilentlyContinue
    Remove-Item Env:WEEKLINE_BOOTSTRAP_MANAGER_PASSWORD -ErrorAction SilentlyContinue
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPointer)
}

$BinaryPath = '"{0}" --service --config "{1}"' -f $HostExecutable, $ConfigPath
if ($existing) {
    & sc.exe config $ServiceName binPath= $BinaryPath start= auto | Out-Null
}
else {
    New-Service -Name $ServiceName -BinaryPathName $BinaryPath -DisplayName "Weekline Office Host" -Description "Hosts the Weekline employee schedule and supervises Caddy." -StartupType Automatic | Out-Null
}
& sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/15000/restart/60000 | Out-Null

foreach ($port in 80, 443) {
    $ruleName = "Weekline HTTPS $port"
    if (-not (Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue)) {
        New-NetFirewallRule -DisplayName $ruleName -Direction Inbound -Action Allow -Protocol TCP -LocalPort $port -Profile Domain,Private | Out-Null
    }
}

Set-Service -Name $ServiceName -StartupType Automatic
Start-Service -Name $ServiceName

Write-Host "Weekline office host installed and started."
Write-Host "Manager installer: $(Join-Path $ManagerDirectory 'WeeklineManager.msi')"
Write-Host "Manager configuration: $ManagerConfigPath"
Write-Host "Copy the manager directory to each authorized manager computer and run install-manager.ps1 as that user."
Write-Host "Forward office router TCP ports 80 and 443 to this desktop, and point $Domain to the Spectrum public IP."
Write-Host "After DNS and router setup, close all manager apps and run:"
Write-Host "  & '$(Join-Path $InstallRoot 'verify-office-deployment.ps1')' -Domain '$Domain'"
