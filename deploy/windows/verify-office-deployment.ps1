[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Domain,

    [string]$InstallRoot = "$env:ProgramData\Weekline",
    [string]$HostControlToken = "",
    [string]$BaseUrl = "",
    [switch]$SkipLocalHostChecks
)

$ErrorActionPreference = "Stop"
$ServiceName = "WeeklineHost"
if (-not $BaseUrl) {
    $BaseUrl = "https://$Domain"
}
$BaseUrl = $BaseUrl.TrimEnd("/")
$BaseUri = [Uri]$BaseUrl
if ($Domain -ne "localhost" -and [Uri]::CheckHostName($Domain) -ne [UriHostNameType]::Dns) {
    throw "Domain must be a DNS hostname without a scheme, port, or path."
}
if ($BaseUri.Scheme -ne "https" -and -not $BaseUri.IsLoopback) {
    throw "BaseUrl must use HTTPS except when testing on loopback."
}
$LeaseA = [Guid]::NewGuid().ToString()
$LeaseB = [Guid]::NewGuid().ToString()

function Assert-True([bool]$Condition, [string]$Message) {
    if (-not $Condition) {
        throw $Message
    }
}

function Write-Pass([string]$Message) {
    Write-Host "PASS  $Message" -ForegroundColor Green
}

function Read-Json([string]$Body, [string]$Context) {
    try {
        return $Body | ConvertFrom-Json
    }
    catch {
        throw "$Context returned invalid JSON."
    }
}

Add-Type -AssemblyName System.Net.Http
$HttpHandler = New-Object System.Net.Http.HttpClientHandler
$HttpHandler.UseCookies = $false
$HttpClient = New-Object System.Net.Http.HttpClient($HttpHandler)
$HttpClient.Timeout = [TimeSpan]::FromSeconds(20)

function Invoke-WeeklineRequest(
    [string]$Method,
    [string]$Path,
    [hashtable]$Headers = @{},
    [string]$JsonBody = ""
) {
    $request = New-Object System.Net.Http.HttpRequestMessage(
        (New-Object System.Net.Http.HttpMethod($Method)),
        "$BaseUrl$Path"
    )
    try {
        foreach ($entry in $Headers.GetEnumerator()) {
            [void]$request.Headers.TryAddWithoutValidation($entry.Key, [string]$entry.Value)
        }
        [void]$request.Headers.TryAddWithoutValidation("Cache-Control", "no-cache, no-store")
        if ($JsonBody) {
            $request.Content = New-Object System.Net.Http.StringContent(
                $JsonBody,
                [Text.Encoding]::UTF8,
                "application/json"
            )
        }
        $response = $HttpClient.SendAsync($request).GetAwaiter().GetResult()
        try {
            return [PSCustomObject]@{
                StatusCode = [int]$response.StatusCode
                Body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
            }
        }
        finally {
            $response.Dispose()
        }
    }
    finally {
        $request.Dispose()
    }
}

function Assert-Status($Response, [int]$Expected, [string]$Context) {
    Assert-True ($Response.StatusCode -eq $Expected) "$Context returned HTTP $($Response.StatusCode); expected $Expected. Body: $($Response.Body)"
    Write-Pass "$Context returned HTTP $Expected"
}

function Invoke-Lease([string]$InstanceId, [string]$MachineName) {
    $body = @{
        machineName = $MachineName
        appVersion = "office-acceptance"
    } | ConvertTo-Json -Compress
    return Invoke-WeeklineRequest `
        -Method "PUT" `
        -Path "/api/host/leases/$InstanceId" `
        -Headers @{ Authorization = "Bearer $HostControlToken" } `
        -JsonBody $body
}

function Remove-Lease([string]$InstanceId) {
    return Invoke-WeeklineRequest `
        -Method "DELETE" `
        -Path "/api/host/leases/$InstanceId" `
        -Headers @{ Authorization = "Bearer $HostControlToken" }
}

try {
    if (-not $HostControlToken) {
        $managerConfigPath = Join-Path $InstallRoot "manager\manager.json"
        Assert-True (Test-Path $managerConfigPath -PathType Leaf) "Manager configuration not found: $managerConfigPath"
        $managerConfig = Get-Content $managerConfigPath -Raw | ConvertFrom-Json
        $HostControlToken = [string]$managerConfig.hostControlToken
    }
    Assert-True ($HostControlToken.Length -ge 32) "The host control token must contain at least 32 characters."

    if (-not $SkipLocalHostChecks) {
        $service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
        Assert-True ($null -ne $service) "The $ServiceName service is not installed."
        Assert-True ($service.Status -eq "Running") "The $ServiceName service is not running."
        Write-Pass "$ServiceName service is running"

        $serviceConfig = Get-CimInstance Win32_Service -Filter "Name='$ServiceName'"
        Assert-True ($serviceConfig.StartMode -eq "Auto") "The $ServiceName service startup mode is $($serviceConfig.StartMode), not Automatic."
        Write-Pass "$ServiceName service starts automatically"

        foreach ($path in @(
            (Join-Path $InstallRoot "bin\weekline-host.exe"),
            (Join-Path $InstallRoot "bin\caddy.exe"),
            (Join-Path $InstallRoot "Caddyfile"),
            (Join-Path $InstallRoot "weekline.env"),
            (Join-Path $InstallRoot "web\index.html")
        )) {
            Assert-True (Test-Path $path -PathType Leaf) "Required installed file is missing: $path"
        }
        Write-Pass "required host files are installed"

        foreach ($port in 80, 443) {
            $rule = Get-NetFirewallRule -DisplayName "Weekline HTTPS $port" -ErrorAction SilentlyContinue
            Assert-True ($null -ne $rule -and $rule.Enabled -eq "True") "The inbound Weekline firewall rule for TCP $port is missing or disabled."
        }
        Write-Pass "Windows firewall rules for TCP 80 and 443 are enabled"
    }

    $addresses = @(
        [Net.Dns]::GetHostAddresses($Domain) |
            Where-Object { $_.AddressFamily -eq [Net.Sockets.AddressFamily]::InterNetwork } |
            ForEach-Object { $_.IPAddressToString }
    )
    Assert-True ($addresses.Count -gt 0) "$Domain did not resolve to an IPv4 address."
    Write-Pass "$Domain resolves to $($addresses -join ', ')"

    $health = Invoke-WeeklineRequest -Method "GET" -Path "/api/health"
    Assert-Status $health 200 "API health check"

    $initialAvailability = Invoke-WeeklineRequest -Method "GET" -Path "/api/host/availability"
    Assert-True ($initialAvailability.StatusCode -eq 503) "At least one manager lease is already active. Close every manager app before running acceptance verification."
    Write-Pass "employee site starts offline with zero manager leases"
    Assert-Status (Invoke-WeeklineRequest -Method "GET" -Path "/?acceptance=$LeaseA") 503 "Employee site with zero leases"

    $status = Invoke-Lease -InstanceId $LeaseA -MachineName "Acceptance-A"
    Assert-Status $status 200 "First manager lease"
    $state = Read-Json $status.Body "First manager lease"
    Assert-True ($state.available -and $state.activeCount -eq 1) "First lease did not produce available=true and activeCount=1."
    Write-Pass "first manager enables the employee site"
    Assert-Status (Invoke-WeeklineRequest -Method "GET" -Path "/?acceptance=$LeaseA") 200 "Employee site with one lease"

    $status = Invoke-Lease -InstanceId $LeaseB -MachineName "Acceptance-B"
    Assert-Status $status 200 "Second manager lease"
    $state = Read-Json $status.Body "Second manager lease"
    Assert-True ($state.available -and $state.activeCount -eq 2) "Second lease did not produce available=true and activeCount=2."
    Write-Pass "second manager holds an independent lease"

    $status = Remove-Lease -InstanceId $LeaseA
    Assert-Status $status 200 "First manager release"
    $state = Read-Json $status.Body "First manager release"
    Assert-True ($state.available -and $state.activeCount -eq 1) "Releasing the first lease did not leave available=true and activeCount=1."
    Assert-Status (Invoke-WeeklineRequest -Method "GET" -Path "/?acceptance=$LeaseB") 200 "Employee site after first manager closes"
    Write-Pass "closing one manager leaves the employee site online"

    $status = Remove-Lease -InstanceId $LeaseB
    Assert-Status $status 200 "Final manager release"
    $state = Read-Json $status.Body "Final manager release"
    Assert-True ((-not $state.available) -and $state.activeCount -eq 0) "Final release did not produce available=false and activeCount=0."
    Assert-Status (Invoke-WeeklineRequest -Method "GET" -Path "/?acceptance=complete") 503 "Employee site after final manager closes"
    Write-Pass "closing the final manager takes the employee site offline"

    Assert-Status (Invoke-WeeklineRequest -Method "GET" -Path "/api/v1/attendance") 404 "Disabled attendance endpoint"

    Write-Host ""
    Write-Host "Weekline office deployment acceptance PASSED." -ForegroundColor Green
}
finally {
    try { [void](Remove-Lease -InstanceId $LeaseA) } catch { }
    try { [void](Remove-Lease -InstanceId $LeaseB) } catch { }
    $HttpClient.Dispose()
    $HttpHandler.Dispose()
}
