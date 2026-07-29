#!/bin/bash

set -u

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
API_PID=""
WEB_PID=""

cleanup() {
  trap - INT TERM EXIT
  [[ -n "$API_PID" ]] && kill "$API_PID" 2>/dev/null
  [[ -n "$WEB_PID" ]] && kill "$WEB_PID" 2>/dev/null
  [[ -n "$API_PID" ]] && wait "$API_PID" 2>/dev/null
  [[ -n "$WEB_PID" ]] && wait "$WEB_PID" 2>/dev/null
}

shutdown() {
  cleanup
  exit 0
}

trap shutdown INT TERM
trap cleanup EXIT

if lsof -nP -iTCP:8080 -sTCP:LISTEN >/dev/null 2>&1; then
  printf '%s\n' "Port 8080 is already in use. Stop the existing API before starting Weekline." >&2
  exit 1
fi
if lsof -nP -iTCP:4200 -sTCP:LISTEN >/dev/null 2>&1; then
  printf '%s\n' "Port 4200 is already in use. Stop the existing frontend before starting Weekline." >&2
  exit 1
fi

if [[ ! -x "$ROOT_DIR/web/node_modules/.bin/ng" ]]; then
  printf '%s\n' "Angular dependencies are missing. Run 'cd web && pnpm install' first." >&2
  exit 1
fi

GO_TEMP_ROOT="${TMPDIR:-/tmp}/weekline-go"
mkdir -p "$GO_TEMP_ROOT/path" "$GO_TEMP_ROOT/modules" "$GO_TEMP_ROOT/cache"

(
  cd "$ROOT_DIR/api" || exit 1
  exec env GOPATH="$GO_TEMP_ROOT/path" \
    GOMODCACHE="$GO_TEMP_ROOT/modules" \
    GOCACHE="$GO_TEMP_ROOT/cache" \
    WEEKLINE_SEED_DEMO="${WEEKLINE_SEED_DEMO:-true}" \
    WEEKLINE_HOST_CONTROL_TOKEN="${WEEKLINE_HOST_CONTROL_TOKEN:-weekline-local-development-control-token}" \
    go run ./cmd/server
) &
API_PID=$!

(
  cd "$ROOT_DIR/web" || exit 1
  exec "$ROOT_DIR/web/node_modules/.bin/ng" serve --host 127.0.0.1
) &
WEB_PID=$!

printf '\n%s\n%s\n\n' \
  "Weekline is starting at http://127.0.0.1:4200" \
  "Press Ctrl+C once to stop both servers."

while kill -0 "$API_PID" 2>/dev/null && kill -0 "$WEB_PID" 2>/dev/null; do
  sleep 1
done

printf '%s\n' "A Weekline process stopped; shutting down the other process." >&2
exit 1
