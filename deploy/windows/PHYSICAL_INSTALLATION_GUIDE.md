# Weekline physical installation guide

This guide covers the first production installation on the office Windows host, PostgreSQL setup, Cloudflare DNS and Tunnel configuration, manager-computer installation, verification, and backups.

## What is installed where

```text
Office Windows host
  PostgreSQL service       127.0.0.1:5432 only
  WeeklineHost Go service  127.0.0.1:8080 only
  Caddy                    127.0.0.1:8081 only
  WeeklineTunnel           outbound Cloudflare Tunnel service
  Angular website          served by Caddy

Manager computers
  Weekline Manager desktop application
  manager.json connection/control configuration
  No PostgreSQL installation

Employee phones/computers
  Ordinary web browser
  No Weekline installation and no PostgreSQL installation
```

Every manager application and employee browser connects to one address:

```text
https://schedule.example.com
```

They never connect directly to PostgreSQL. Cloudflare terminates public HTTPS and forwards requests through the named tunnel to Caddy on the same host. Caddy sends `/api` requests to the Go service, and the Go service connects to PostgreSQL. No Weekline port is forwarded through the router.

If the office host, its power, or its Internet service fails, remote Weekline access fails because this design deliberately uses no rented cloud server.

## Why PostgreSQL is currently a separate installation

The present Weekline release contains the Weekline website, Go service, Caddy, `cloudflared`, Windows service installers, manager MSI, and verification tools. It does **not** contain the PostgreSQL Windows installer.

PostgreSQL is a separate long-running database service with its own data directory, Windows service, security updates, backups, and major-version upgrades. Keeping it as an explicit prerequisite avoids an application update silently replacing or deleting the database. The PostgreSQL project recommends binary packages where available, and its Windows download page offers a graphical installer containing the server and command-line tools:

- <https://www.postgresql.org/download/windows/>
- <https://www.postgresql.org/docs/current/install-binaries.html>

PostgreSQL does provide binary archives intended for vendors that embed it, so a future unified Weekline bootstrapper is possible. That packaging has not been implemented; do not assume PostgreSQL is inside the current Weekline release.

## Values to collect before starting

Write these down in an administrator password manager, not in an email:

| Item | Example | Your value |
|---|---|---|
| Public schedule hostname | `schedule.example.com` | |
| Public schedule hostname | `schedule.example.com` | |
| Cloudflare Tunnel token | copied from the named tunnel dashboard page | |
| PostgreSQL administrator | `postgres` | |
| PostgreSQL administrator password | created during PostgreSQL installation | |
| Weekline database | `weekline` | |
| Weekline database role | `weekline` | |
| Weekline database password | unique random password | |
| Initial Weekline manager email | `manager@example.com` | |
| Initial Weekline manager name | `Office Manager` | |
| Initial Weekline manager password | unique 12-72 byte password | |

The PostgreSQL administrator account, Weekline database role, and Weekline manager login are three different credentials.

## Phase 1: prepare the Cloudflare account and organizational domain

The WordPress website and the schedule hostname can share one organizational domain. WordPress remains at its current host; only a new `schedule` subdomain is routed through Cloudflare Tunnel.

1. Log in to the organization-owned domain registrar or DNS provider and record every existing DNS record before changing anything.
2. Add the organizational domain to a free Cloudflare account. If Cloudflare asks to change nameservers, copy every WordPress-related DNS record first so the existing website and email continue to work.
3. In Cloudflare, create a **named, remotely managed** tunnel named `weekline-office-host`.
4. Add a public hostname such as `schedule.example.com`. Set its service/origin to `http://127.0.0.1:8081`.
5. Copy the generated tunnel installation token into a password manager. Anyone holding this token can run the tunnel, so do not email it or commit it to GitHub.

Cloudflare’s current Windows Tunnel setup is documented at <https://developers.cloudflare.com/tunnel/setup/>. Do not use a temporary Quick Tunnel for production because it has no stable organizational hostname or uptime commitment.

## Phase 2: prepare the Windows office host

Use a supported Windows 11 desktop or Windows Server computer. Prefer wired Ethernet and connect it to a UPS if possible.

### 2.1 Update and identify the host

1. Install current Windows security updates.
2. Give the computer a recognizable name such as `WEEKLINE-HOST`.
3. Connect it by Ethernet.
4. Open PowerShell as Administrator and run:

```powershell
ipconfig /all
Get-NetConnectionProfile
```

Record the active Ethernet adapter's IPv4 address, physical/MAC address, subnet mask, default gateway, and DNS servers.

### 2.2 Reserve its LAN address

The preferred method is a DHCP reservation in the router:

```text
Host MAC address -> 192.168.1.50
```

Choose an unused address in the existing LAN subnet. Do not copy the example if the office uses a different subnet. A DHCP reservation avoids accidental address conflicts and makes the office host easier to identify; Cloudflare Tunnel does not require that address to be publicly reachable.

After creating the reservation, renew the address or restart the host, then confirm:

```powershell
ipconfig
```

### 2.3 Use a Private or Domain Windows network profile

The Weekline installer does not create inbound HTTP or HTTPS firewall rules. Keep the host on a Private or Domain network profile so the normal office security baseline applies.

```powershell
Get-NetConnectionProfile
Set-NetConnectionProfile -InterfaceAlias "Ethernet" -NetworkCategory Private
```

Replace `Ethernet` with the actual interface alias. Do not disable Windows Firewall; Microsoft recommends keeping it enabled: <https://learn.microsoft.com/en-us/windows/security/operating-system-security/network-security/windows-firewall/>.

### 2.4 Prevent sleep and hibernation

In an elevated PowerShell window:

```powershell
powercfg /change standby-timeout-ac 0
powercfg /hibernate off
```

Microsoft documents `powercfg /hibernate off` and the other `powercfg` options here:

- <https://learn.microsoft.com/en-us/troubleshoot/windows-client/setup-upgrade-and-drivers/disable-and-re-enable-hibernation>
- <https://learn.microsoft.com/en-us/windows-hardware/design/device-experiences/powercfg-command-line-options>

Also open **Settings > System > Power** and verify that the computer never sleeps while plugged in. The display may turn off; the computer itself must remain awake.

## Phase 3: install PostgreSQL on the office host

### 3.1 Download PostgreSQL

Use the Windows installer linked by the PostgreSQL project:

<https://www.postgresql.org/download/windows/>

Use a supported 64-bit version. PostgreSQL 17 is the conservative Weekline baseline; the application supports PostgreSQL 16 or newer. Do not install a beta or release-candidate build.

### 3.2 Run the PostgreSQL installer

During the graphical installation:

1. Keep **PostgreSQL Server** selected.
2. Keep **Command Line Tools** selected.
3. pgAdmin is optional but useful.
4. Use the default installation and data directories unless the office has a planned data drive.
5. Set a strong password for the PostgreSQL `postgres` administrator account and store it in the administrator password manager.
6. Keep TCP port `5432`.
7. Use the default locale unless the organization requires another locale.
8. Stack Builder and extra extensions are not required for Weekline.

The installer creates a PostgreSQL Windows service. Confirm it is running:

```powershell
Get-Service postgresql*
```

### 3.3 Restrict PostgreSQL to the host itself

Open **SQL Shell (psql)** from the Start menu. Connect using:

```text
Server: localhost
Database: postgres
Port: 5432
Username: postgres
Password: the administrator password from installation
```

Find the exact configuration paths:

```sql
SHOW config_file;
SHOW hba_file;
```

Open `postgresql.conf` as Administrator and ensure:

```conf
listen_addresses = 'localhost'
port = 5432
password_encryption = 'scram-sha-256'
```

In `pg_hba.conf`, ensure local TCP connections use SCRAM:

```conf
host    all    all    127.0.0.1/32    scram-sha-256
host    all    all    ::1/128         scram-sha-256
```

Do not add `0.0.0.0/0`, the host's LAN subnet, or a public address. PostgreSQL documents SCRAM password authentication and recommends moving away from MD5:

- <https://www.postgresql.org/docs/18/auth-password.html>
- <https://www.postgresql.org/about/featurematrix/detail/scram-sha-256-authentication/>

Restart PostgreSQL after editing. Obtain the exact service name from `Get-Service postgresql*`:

```powershell
Restart-Service -Name "postgresql-x64-17"
```

The actual suffix may differ.

### 3.4 Create the dedicated Weekline database account

Reconnect with SQL Shell as the `postgres` administrator. Run:

```sql
CREATE ROLE weekline
  LOGIN
  NOSUPERUSER
  NOCREATEDB
  NOCREATEROLE
  NOREPLICATION;

\password weekline
```

At the two password prompts, enter the dedicated Weekline database password. A long randomly generated hexadecimal password is easiest because it does not require URL encoding.

Then run:

```sql
CREATE DATABASE weekline
  WITH OWNER weekline
  ENCODING 'UTF8'
  TEMPLATE template0;

REVOKE ALL ON DATABASE weekline FROM PUBLIC;
GRANT CONNECT, TEMPORARY ON DATABASE weekline TO weekline;
```

The role is deliberately not a superuser and cannot create other databases or roles. PostgreSQL warns that superuser access should not be granted casually: <https://www.postgresql.org/docs/current/app-createuser.html>.

Test the account from PowerShell, adjusting the versioned path if necessary:

```powershell
& "C:\Program Files\PostgreSQL\17\bin\psql.exe" `
  -h 127.0.0.1 `
  -p 5432 `
  -U weekline `
  -d weekline `
  -W `
  -c "SELECT current_database(), current_user;"
```

Expected values are `weekline` and `weekline`.

### 3.5 Construct the database connection URL

For a hexadecimal password, the connection URL is:

```text
postgres://weekline:YOUR_HEX_PASSWORD@127.0.0.1:5432/weekline?sslmode=disable
```

`sslmode=disable` is acceptable here only because PostgreSQL is bound to loopback and the Go service is on the same computer. PostgreSQL is never traversing the LAN or Internet.

If the password contains characters such as `@`, `:`, `/`, `?`, `#`, or `%`, percent-encode it before placing it in a URI. PostgreSQL requires URI components containing reserved characters to be percent-encoded: <https://www.postgresql.org/docs/16/libpq-connect.html>.

PowerShell example:

```powershell
$EncodedPassword = [Uri]::EscapeDataString("YOUR_DATABASE_PASSWORD")
$DatabaseUrl = "postgres://weekline:$EncodedPassword@127.0.0.1:5432/weekline?sslmode=disable"
```

Do not print or save `$DatabaseUrl` in an ordinary text document.

## Phase 4: verify the Cloudflare public hostname

Cloudflare creates the required DNS route when the public hostname is added to the named tunnel. It must point at the tunnel, not the office public IP.

Verify that the hostname resolves and that it is associated with the tunnel in the Cloudflare dashboard:

```powershell
Resolve-DnsName schedule.example.com
```

The dashboard should show the `weekline-office-host` tunnel as **Healthy** only after the office host installation has started the `WeeklineTunnel` service.

## Phase 5: remove legacy public exposure

Do not configure or retain router forwarding for TCP ports 80 or 443. The office host starts an outbound encrypted connection to Cloudflare, so a public IPv4, static IP address, dynamic DNS, NAT loopback, and inbound Spectrum rules are not required for Weekline.

Confirm that no old rules remain:

```text
TCP WAN 80  -> no destination
TCP WAN 443 -> no destination
```

Keep all of these services private:

- PostgreSQL `5432`
- Go API `8080`
- Caddy administration `2019`
- Remote Desktop `3389`

## Phase 6: install Weekline on the host

The Weekline Windows release must already have been built. Copy the entire generated `dist\windows` folder from the build computer to a local folder on the host, for example:

```text
C:\Weekline-Release\
```

Do not run the installer directly from a removable drive or network share. Copy it locally first.

Open PowerShell as Administrator:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
Set-Location C:\Weekline-Release

$EncodedPassword = [Uri]::EscapeDataString("YOUR_DATABASE_PASSWORD")
$DatabaseUrl = "postgres://weekline:$EncodedPassword@127.0.0.1:5432/weekline?sslmode=disable"

.\install-office-host.ps1 `
  -Domain schedule.example.com `
  -DatabaseUrl $DatabaseUrl `
  -CloudflareTunnelToken "PASTE_THE_TOKEN_FROM_CLOUDFLARE_HERE" `
  -ManagerEmail manager@example.com `
  -ManagerName "Office Manager"
```

The installer securely prompts for the first Weekline manager password. That password is used once to create the first manager account and is not stored in `weekline.env` or `manager.json`.

The installer then:

1. Copies the website, Go server, Caddy, `cloudflared`, verifier, and manager package into `%ProgramData%\Weekline`.
2. Stores the PostgreSQL connection URL in `%ProgramData%\Weekline\weekline.env` with access restricted to SYSTEM and Administrators.
3. Connects to PostgreSQL and automatically applies every Weekline database migration.
4. Creates the first Weekline manager if no manager exists.
5. Creates the automatic `WeeklineHost` service.
6. Configures service recovery after failures.
7. Removes any legacy Weekline inbound firewall rules for TCP 80 and 443.
8. Starts `WeeklineHost`; that service starts the localhost-only Caddy origin.
9. Creates and starts `WeeklineTunnel`, which uses the Cloudflare token to maintain the outbound public path.
10. Generates the manager-computer configuration and secret control token.

Confirm both services:

```powershell
Get-Service WeeklineHost
Get-Service WeeklineTunnel
Get-Service postgresql*
```

Both must report `Running`. Do not display or share `%ProgramData%\Weekline\weekline.env`.

If Cloudflare was not configured when the host started, finish the public-hostname setup and restart the tunnel service:

```powershell
Restart-Service WeeklineTunnel
```

## Phase 7: verify the host and public path

### 7.1 Basic checks

```powershell
Resolve-DnsName schedule.example.com
Invoke-RestMethod https://schedule.example.com/api/health
```

The health endpoint should return `status: ok` even when no manager app is open.

Opening `https://schedule.example.com/` before a manager opens the desktop app should return HTTP 503. That is expected: the controller is ready, but the employee website is deliberately gated off.

### 7.2 Automated acceptance

Close all real Weekline Manager applications. In elevated PowerShell:

```powershell
& "$env:ProgramData\Weekline\verify-office-deployment.ps1" `
  -Domain schedule.example.com
```

Require the final message:

```text
Weekline office deployment acceptance PASSED.
```

This checks both services, files, the absence of legacy inbound firewall rules, DNS, HTTPS health, disabled attendance route, and the `0 -> 1 -> 2 -> 1 -> 0` temporary manager-lease sequence.

### 7.3 Test from outside the office

Turn off Wi-Fi on a phone and use cellular data. Open:

```text
https://schedule.example.com/api/health
```

If it does not work over cellular, check the Cloudflare dashboard for the tunnel’s health, confirm the public hostname points to the named tunnel, and confirm that `WeeklineTunnel` is running. Router NAT, static-IP status, and CGNAT are not part of this path.

## Phase 8: install each manager computer

After host installation, this directory exists:

```text
C:\ProgramData\Weekline\manager\
  WeeklineManager.msi
  install-manager.ps1
  manager.json
```

Copy the entire `manager` directory to an encrypted USB drive. Treat `manager.json` as a secret because it contains the host-control token.

On each authorized manager computer:

1. Sign in as the Windows user who will operate Weekline.
2. Copy the complete folder from USB to a local folder.
3. Open a normal, non-administrator PowerShell window in that folder.
4. Run:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\install-manager.ps1
```

The script requests elevation only for the MSI. It then returns to the signed-in user's context, writes `%APPDATA%\Weekline\manager.json`, and restricts the file to that user, SYSTEM, and Administrators.

No PostgreSQL installation is required on a manager computer. The manager application connects to `https://schedule.example.com` through the office server.

Complete the physical lifecycle test:

1. With both manager apps closed, employee site is unavailable.
2. Open manager A; employee site becomes available.
3. Open manager B; site remains available.
4. Close manager A; site remains available because B still has a lease.
5. Close manager B; site becomes unavailable immediately after its clean release.
6. Force-close a manager app once; site becomes unavailable after the 45-second lease expiry if no other manager remains.

## Phase 9: employee access

Employees do not install anything. They open the public hostname in a modern browser and sign in with the worker accounts created by the Weekline manager.

Only the manager desktop app controls whether the employee website is available. PostgreSQL, Caddy, and the private controller remain running so any authorized manager computer can reactivate the site.

## Phase 10: backups

PostgreSQL must be backed up independently of the Weekline program files. PostgreSQL recommends regular backups and provides `pg_dump` for consistent logical backups:

- <https://www.postgresql.org/docs/17/backup.html>
- <https://www.postgresql.org/docs/17/app-pgdump.html>

Create a backup directory on an encrypted external drive or another protected device. A manual custom-format backup is:

```powershell
$Stamp = Get-Date -Format "yyyyMMdd-HHmmss"
& "C:\Program Files\PostgreSQL\17\bin\pg_dump.exe" `
  -h 127.0.0.1 `
  -U weekline `
  -d weekline `
  -F c `
  -f "E:\WeeklineBackups\weekline-$Stamp.dump" `
  -W
```

Store at least one backup away from the host. A backup kept only on the same desktop is lost if that disk fails or the computer is stolen. Test restoration periodically on a separate database or test computer.

## Troubleshooting map

| Symptom | Most likely area |
|---|---|
| PostgreSQL test login fails | PostgreSQL service, role password, `pg_hba.conf`, or database ownership |
| WeeklineHost will not stay running | Database URL, PostgreSQL availability, configuration permissions, or Caddy startup |
| WeeklineTunnel will not stay running | Invalid or rotated Cloudflare tunnel token, blocked outbound Internet access, or `cloudflared` startup failure |
| `/api/health` works but `/` returns 503 | Expected when no manager lease is active |
| Manager opens but cannot enable site | Wrong `manager.json`, HTTPS/DNS failure, or host-control token mismatch |
| Works on host but not cellular | Tunnel is unhealthy, public hostname is not attached to the tunnel, or the Cloudflare DNS change has not propagated |
| Browser reports certificate error | The schedule hostname is not proxied through Cloudflare, DNS points elsewhere, or the local system clock is wrong |
| Manager computer asks for PostgreSQL | Incorrect package or configuration; managers must never connect to PostgreSQL |

## Security rules that must remain true

- Do not publicly expose or forward any office-host ports.
- Keep PostgreSQL 5432, API 8080, Caddy admin 2019, and Remote Desktop 3389 private.
- Keep PostgreSQL bound to localhost.
- Keep Windows Firewall enabled.
- Protect `manager.json`, `weekline.env`, database passwords, and backups.
- Apply Windows, PostgreSQL, and Weekline security updates.
- Use a UPS and disable host sleep.
- Do not use the PostgreSQL `postgres` superuser as Weekline's application account.
- Do not install PostgreSQL on manager or employee computers.
