# Weekline

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](../LICENSE)

Weekline is a responsive employee scheduling application intended to replace a weekly Excel-and-paper workflow. Managers create and publish schedules from a desktop-oriented weekly grid; workers use a focused mobile view to see their shifts and coworkers assigned on the same day.

## Architecture

- Angular 22 standalone frontend with no additional UI libraries
- Go HTTP API using the standard library router
- PostgreSQL persistence for schedules, requests, leave, availability, sessions, and audit events
- Caddy for HTTPS, static frontend hosting, and `/api` reverse proxying
- First-party Deno Desktop 2.9+ with the CEF backend for the manager application
- macOS development with deployment artifacts suitable for Windows or Linux

Authentication uses server-side sessions stored in PostgreSQL and an HTTP-only, SameSite cookie. Manager and worker routes are separately guarded in both Angular and the Go API. Published shifts are snapshotted so managers can edit a new draft without changing what workers currently see.

## Local development

Prerequisites: Node 26.10.0 (the repository includes an `.nvmrc`), pnpm 12.8.1, Go 1.27.1+, and PostgreSQL 18.6 (older supported versions require separate validation).

Angular's optional persistent build cache is disabled because its native cache module crashes on the current macOS environment. Compilation and runtime behavior are unaffected; builds simply do not reuse that disk cache.

First-time setup:

```sh
brew services start postgresql@18
createdb weekline_dev
cd web && pnpm install && cd ..
```

From the repository root, start both servers in one terminal:

```sh
./dev.sh
```

Open `http://127.0.0.1:4200`. Keep that terminal visible. Press `Ctrl+C` once to stop both the API and frontend; the script also stops the remaining process if either server exits unexpectedly.

Tests and production builds can be run without starting the development servers:

```sh
cd api && GOCACHE="${TMPDIR:-/tmp}/weekline-go-cache" go test ./...
cd ../web && ./node_modules/.bin/ng build
```

The local seed creates manager and worker demo accounts; use the buttons on the login screen to fill either account. Shift cells open the editor, blank cells create a database record, and **Publish schedule** promotes the draft for workers.

Production must set `WEEKLINE_DATABASE_URL`, enable `WEEKLINE_COOKIE_SECURE=true`, and omit `WEEKLINE_SEED_DEMO`.
The Windows host installer securely bootstraps the first production manager on a new database; demo accounts are not used in production.

The no-cloud Windows office-host build, installation, and post-install acceptance procedure is documented in [`deploy/windows/README.md`](../deploy/windows/README.md).

## Windows downloads

A Windows release is a ZIP archive, not only the manager MSI. The ZIP contains
the compiled website, `weekline-host.exe`, Caddy, `WeeklineManager.msi`, the
host and manager installers, the deployment verifier, and a version file.

Tagged builds are published on the repository's **Releases** page as
`Weekline-windows-<version>.zip`. The office administrator downloads and
extracts that ZIP on the designated host, then follows
[`deploy/windows/README.md`](../deploy/windows/README.md). PostgreSQL is installed
separately from its official Windows installer because it owns durable data and
must survive Weekline application upgrades.

After the office-host installer runs, it creates a private manager directory
containing the generic MSI and an office-specific `manager.json`. That private
directory is copied directly to authorized manager computers; it must never be
uploaded to GitHub because it contains the host-control token.

The source code is licensed under the [MIT License](../LICENSE). Release binaries
are initially unsigned and may display an Unknown Publisher warning on Windows;
code signing is required before broad distribution outside the organization.

## Implemented workforce foundation

- Draft/published weekly schedules, worker views, printing, people status, and audit history
- Direct shift swaps, open-shift claims, manager decisions, and expiration/cancellation states
- Leave types, balances, approvals, schedule blocking, and public holidays
- Coverage requirements, open positions, assignment suggestions, and worker claims
- Central conflict validation for overlaps, leave, eligibility, overtime, rest, minimum length, published edits, and availability
- Recurring unavailable/preferred hours with manager schedule guidance
- Drag-and-drop shift move, Option/Alt-copy, and occupied-cell swap on the manager schedule

Attendance and clocking are deliberately disabled because another system handles them. Also out of scope are shift templates, recurring/rotating/split schedule generation, bulk assignment, multi-position shifts, expanded organization hierarchies, granular permission groups, notifications, SSO, mobile punching, GPS, geolocation, customer job costing, equipment scheduling, and payroll calculation/export.

Migration compatibility: Angular 22.2 requires TypeScript >=6.0 <6.1, so TypeScript 6.0.3 is held. Jasmine 6.3.0/types 6.0.0 are held because Jasmine 7 is incompatible with Zone.js 0.16.3’s Jasmine adapter (read-only `describe` globals). Chromium tests pass with Jasmine 6. Deno is pinned to 2.9.7 in `.deno-version`.

## Migration validation

Validated on Node 26.10.0/pnpm 12.8.1/Go 1.27.1/Deno 2.9.7, PostgreSQL 18.6, and Caddy 2.11.4. PostgreSQL 19 is still a beta and is excluded. Clean frontend install/build/audit, Chromium tests, Go tests/vet, Deno tests/format, and production API/Angular desktop-manager/mobile-worker checks passed. Disposable PostgreSQL fixtures covered authentication, manager/worker role restrictions, shift creation/editing, publication snapshots, supporting resources, and logout. Caddy configuration validation and real local API proxy/SPA fallback passed.

Windows AMD64 host cross-compilation and CEF manager MSI packaging succeeded on macOS. Native Windows installation, office LAN/TLS setup, service behavior, and MSI launch remain unverified and must follow the Windows deployment acceptance guide before real deployment. No deployment or public release artifacts were produced by these local checks. Existing local PostgreSQL databases were not upgraded or changed.
