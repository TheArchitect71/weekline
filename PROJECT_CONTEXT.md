# Weekline project context

This file is the durable handoff for future Codex tasks after the project folder is moved or a task disappears from the sidebar. Open this repository and give Codex this file together with the current request.

## Product boundary

Weekline replaces the weekly Excel-and-paper schedule workflow. One manager maintains the schedule; employees sign in to a responsive website to see published shifts. The manager experience is a Deno desktop application.

Required now:

- Draft, review, and publish weekly schedules.
- Employee schedule website.
- Manager drag-and-drop move, Option/Alt-copy, and occupied-cell swap.
- People, requests, time off, coverage, availability, history, and conflict checks already present in the application.
- Self-hosting on the office Windows desktop with no rented cloud hardware.
- Multiple manager desktop apps may be open. Each holds an independent renewable lease. Closing one must not take the employee site down while another lease remains. Releasing or losing the last lease makes the employee site and worker API unavailable.

Not being developed:

- Shift templates, recurrence, rotations, split shifts, bulk generation/assignment, or multi-position shifts.
- Expanded locations, teams, departments, reporting hierarchy, custom fields, or granular permission groups. There is one manager.
- Notifications, email alerts, or chat alerts.
- LDAP, OIDC, or SSO.
- Attendance, clock-in/out, kiosk/mobile punching, device identification, GPS, or geolocation. Separate software handles timekeeping. Legacy attendance code and migrations may remain in the repository, but routes are disabled by default and UI entry points are removed.

## Office-host architecture

```text
Employee browser
      |
Internet / HTTPS
      |
Spectrum Enterprise public IPv4
      |
Office router: TCP 80 and 443 only
      |
Windows office desktop
      |-- Caddy: TLS and static Angular site
      |-- Go API/controller: 127.0.0.1:8080
      `-- PostgreSQL: local only, never forwarded

Authorized manager desktops
      `-- Deno Desktop CEF WeeklineManager.msi -> HTTPS lease heartbeat
```

The Windows `WeeklineHost` service, Caddy, and PostgreSQL remain running so a remote manager app can securely reactivate the employee site. Caddy checks `/api/host/availability` before serving the Angular site. Worker login/API access also requires at least one active manager lease in production. This controller-ready state is not considered the employee website being up.

Each manager app uses a UUID lease, renews it every 15 seconds, and has a 45-second server expiry. A normal close releases only that UUID. A crash or network loss expires automatically. Therefore only the final release or expiry disables employee access.

## Spectrum and deployment decisions

- Confirm that the Spectrum Enterprise circuit has a public IPv4 address; a static address is preferred.
- Point the schedule domain to that public IP. A dynamic IP requires dynamic DNS.
- Reserve a LAN address for the office host.
- Verify that the public schedule hostname also reaches the host from the office LAN. If the router does not support NAT loopback, configure a local/router DNS override that resolves the same hostname to the host's reserved LAN address; keep the hostname unchanged so HTTPS certificate validation still works.
- Forward TCP 80 and 443 to the office host. Never forward PostgreSQL 5432 or API 8080.
- Disable sleep and hibernation on the host and allow the `WeeklineHost` service to start automatically after reboot.
- Caddy obtains and renews HTTPS certificates.

Windows build and installation instructions are in `deploy/windows/README.md`.

## Repository map

- `web/`: Angular employee and manager frontend.
- `api/`: Go API, PostgreSQL store, and migrations.
- `desktop/`: First-party Deno Desktop CEF shell and lease heartbeat client.
- `deploy/Caddyfile`: HTTPS/static/reverse-proxy and lease gate.
- `deploy/windows/`: Windows release builder, installer, first-manager bootstrap, post-install acceptance verifier, and complete physical deployment guide.
- `dev.sh`: preferred macOS/local development launcher.

## Verification commands

```sh
cd api
GOPATH=/private/tmp/weekline-go/path \
GOMODCACHE=/private/tmp/weekline-go/modules \
GOCACHE=/private/tmp/weekline-go/cache \
go test ./...

cd ../web
./node_modules/.bin/ng build

cd ..
deno task desktop:check
deno task desktop:test
```

Run the local application with `./dev.sh` and open `http://127.0.0.1:4200`.

The desktop build requires Deno 2.9.0 or newer because `deno desktop` first shipped in 2.9. The production manager package uses the CEF backend for consistent rendering and is emitted as `WeeklineManager.msi`.

## Verified in the current worktree (2026-07-14)

- Go tests and vet, Angular production build, Deno formatting/tests, and Windows Go cross-compilation pass.
- A real PostgreSQL-backed API test verified the `0 -> 1 -> 2 -> 1 -> 0` manager-lease lifecycle, worker login/API gating, final-release shutdown, crash expiry, and disabled attendance route.
- A live Caddy test verified employee-site `503 -> 200 -> 503` behavior around the first and final leases. The Caddy configuration also validates with a Windows web-root path containing spaces.
- Two simultaneous native Deno Desktop CEF applications held distinct leases. Closing the first left the site available; closing the second disabled it.
- The current manager source cross-builds into a valid Windows MSI, and the Windows installer/build scripts parse successfully in PowerShell.
- The packaged PowerShell acceptance verifier passed live against PostgreSQL, the Go controller, and Caddy, proving the same `0 -> 1 -> 2 -> 1 -> 0` lifecycle through the website hostname.
- Windows configuration files are written as UTF-8 without a BOM, and the Go loader also has a regression-tested BOM fallback for compatibility with older PowerShell behavior.
- A fresh production-style database test created exactly one initial manager, rejected a second bootstrap by making no changes, and authenticated the created manager through the real login API with demo seeding disabled.

These checks validate the application and release artifacts, but they do not substitute for installing them on the actual office Windows host and testing the real Spectrum/router path.

## Remaining site-specific work

These steps require the actual office network or Windows computers and cannot be completed on the macOS development machine:

1. Confirm Spectrum static/public IPv4 service.
2. Choose/register the schedule domain and create its DNS record.
3. Install PostgreSQL and the Weekline Windows release on the designated office desktop.
4. Reserve the host LAN address and configure router port forwarding.
5. Disable host sleep/hibernation and verify restart recovery.
6. Install the manager MSI and configuration on every authorized manager computer.
7. Run `verify-office-deployment.ps1` on the office host and require a passing result.
8. Test the employee hostname from a phone using cellular data, not office Wi-Fi.
9. Run a real two-machine acceptance test: open A, open B, close A, confirm site remains available, close B, confirm site becomes unavailable within the lease window.
