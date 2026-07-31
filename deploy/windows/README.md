# Weekline office-host deployment

This deployment keeps all application compute and PostgreSQL data on the designated Windows office desktop. No rented cloud compute is required.

For the complete first-time physical procedure—including PostgreSQL account creation, Cloudflare DNS and Tunnel setup, manager installation, offsite verification, and backups—use [`PHYSICAL_INSTALLATION_GUIDE.md`](PHYSICAL_INSTALLATION_GUIDE.md).

## What gets downloaded, and from where

The office administrator downloads `Weekline-windows-<version>.zip` from the
project's GitHub Releases page. That ZIP is the complete Weekline application
distribution. `WeeklineManager.msi` inside it is only the native Windows
manager application; by itself it does not install the website, API, Caddy, or
database.

PostgreSQL is downloaded separately from the official PostgreSQL Windows page.
It remains a separate Windows service and data directory so installing or
upgrading Weekline cannot silently replace or delete the schedule database.

The public GitHub release never contains credentials. During host installation,
Weekline generates the office-specific database configuration and host-control
token locally. The resulting `%ProgramData%\Weekline\manager` directory is the
private package copied to authorized manager computers.

## Network prerequisites

1. Keep the existing WordPress site at its current host and preserve every existing DNS record.
2. Add the organizational domain to Cloudflare and create a named, remotely managed tunnel.
3. Configure the public `schedule` hostname in Cloudflare with origin `http://127.0.0.1:8081`.
4. Obtain the tunnel token from Cloudflare and store it only on the office host during installation.
5. Do not forward router or Windows firewall ports 80, 443, 5432, or 8080.

## Host prerequisites

- Windows 11 or a supported Windows Server release
- PostgreSQL running locally with a Weekline database and dedicated database account
- Node, pnpm, Go, and the exact Deno version in `.deno-version` on the build computer
- An official Caddy Windows executable supplied to `build-release.ps1`
- An official `cloudflared` Windows executable supplied to `build-release.ps1`
- Sleep and hibernation disabled on the office host
- The office network is configured as a Windows Domain or Private profile
- Windows security updates and automatic service recovery remain enabled

Deno Desktop is experimental. The release build pins Deno through
`.deno-version` and refuses a different version so upgrades cannot silently
change the generated MSI. Rerun the desktop acceptance tests before changing
that pin.

## Build

Run from elevated PowerShell on the build computer:

```powershell
.\deploy\windows\build-release.ps1 `
  -CaddyExecutable C:\path\to\caddy.exe `
  -CloudflaredExecutable C:\path\to\cloudflared.exe
```

## Install the office host

From the generated `dist\windows` directory:

```powershell
.\install-office-host.ps1 `
  -Domain schedule.example.com `
  -DatabaseUrl "postgres://weekline:REPLACE_PASSWORD@127.0.0.1/weekline?sslmode=disable" `
  -CloudflareTunnelToken "PASTE_TUNNEL_TOKEN_HERE"
```

The installer creates the automatic `WeeklineHost` service and a separate automatic `WeeklineTunnel` service. Caddy listens only on `127.0.0.1:8081`; `WeeklineTunnel` is the sole public path. The employee site and worker API are enabled only while at least one authorized manager desktop lease is active.

On a new database, the installer prompts for the initial Weekline manager's email, name, and password. It uses the password only for the one-time account bootstrap and does not write that password to either configuration file.

The installed host's `%ProgramData%\Weekline\manager` directory contains the first-party Deno Desktop CEF installer, the generated configuration, and `install-manager.ps1`. Copy that whole directory to each authorized manager computer. While signed in as the manager who will use Weekline, run:

```powershell
.\install-manager.ps1
```

The script requests elevation only for Windows Installer, then writes `%APPDATA%\Weekline\manager.json` in the manager's own profile and restricts the token file to that user, SYSTEM, and Administrators. Use `-SkipMsiInstall` when only refreshing an existing installation's configuration.

The manager control token is sensitive. Give the manager files only to authorized users and do not put `manager.json` in email or source control.

The generated MSI and executable are unsigned until a Windows code-signing
certificate is configured. Windows therefore labels them **Unknown Publisher**:
the files can still be used for an internal pilot after an administrator
confirms their GitHub release checksum, but Windows cannot cryptographically
identify the publisher. Sign the MSI and packaged executable before broader
distribution.

## Verify the installed office host

After Cloudflare reports the tunnel Healthy and the public hostname is configured, close every real Weekline Manager app. Then run the packaged verifier from an elevated PowerShell window on the office host:

```powershell
& "$env:ProgramData\Weekline\verify-office-deployment.ps1" `
  -Domain schedule.example.com
```

The verifier checks both automatic Windows services, required files, the absence of legacy inbound HTTP/HTTPS rules, public DNS resolution, HTTPS API health, disabled attendance route, and the complete employee-site lease sequence. It creates two temporary acceptance leases, proves `0 -> 1 -> 2 -> 1 -> 0`, and removes both leases in a `finally` block if any check fails. A passing run ends with `Weekline office deployment acceptance PASSED`.

This host-side check does not prove the public Cloudflare path from an independent network. After it passes, disconnect a phone from office Wi-Fi and open the schedule hostname over cellular data. Also complete the real two-computer manager test described below.

## Lifecycle guarantee

- Opening the first manager app creates a 45-second renewable lease and enables the employee site.
- Additional manager apps create independent leases.
- Closing one manager app releases only its lease.
- A crashed or disconnected app expires automatically after 45 seconds.
- The final release or expiry disables the employee site and worker API.
- PostgreSQL, the Windows controller service, and Caddy remain ready so another manager computer can reopen the site without exposing a second control port.

Keeping the small controller alive is intentional. Completely stopping the controller would prevent a manager on another computer from securely waking the site.
