#!/usr/bin/env bash
set -euo pipefail

MODE="${1:-commit}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/pinhaoclaw-frontend"
TMP_DIR="$ROOT_DIR/.tmp/quality-gate"
SMOKE_PORT="${PINHAOCLAW_SMOKE_PORT:-19090}"

log() {
  printf '[quality] %s\n' "$*"
}

fail() {
  printf '[quality] ERROR: %s\n' "$*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing command: $1"
}

check_mode() {
  case "$MODE" in
    commit|push|ci) ;;
    *) fail "unsupported mode: $MODE (expected commit, push or ci)" ;;
  esac
}

check_go_format() {
  log "checking gofmt"
  local unformatted
  unformatted="$(cd "$ROOT_DIR" && find . -path './.git' -prune -o -path './pinhaoclaw-frontend' -prune -o -name '*.go' -print0 | xargs -0r gofmt -l | cat)"
  if [[ -n "$unformatted" ]]; then
    fail "gofmt required for:\n$unformatted"
  fi
}

run_go_tests() {
  log "running go test ./..."
  (cd "$ROOT_DIR" && go test ./...)
}

run_go_race_tests() {
  local goos goarch
  goos="$(cd "$ROOT_DIR" && go env GOOS)"
  goarch="$(cd "$ROOT_DIR" && go env GOARCH)"
  if [[ "$goos" != "linux" && "$goos" != "darwin" && "$goos" != "windows" ]]; then
    log "skipping go test -race on unsupported platform: ${goos}/${goarch}"
    return
  fi

  log "running go test -race ./..."
  (cd "$ROOT_DIR" && go test -race ./...)
}

run_go_build() {
  log "running go build ./..."
  (cd "$ROOT_DIR" && go build ./...)
}

ensure_frontend_deps() {
  if [[ "$MODE" == "ci" || ! -d "$FRONTEND_DIR/node_modules" ]]; then
    log "installing frontend dependencies"
    (cd "$FRONTEND_DIR" && npm ci)
  fi
}

run_frontend_build() {
  ensure_frontend_deps
  log "running npm test"
  (cd "$FRONTEND_DIR" && npm test)
  log "running npm run build:h5"
  (cd "$FRONTEND_DIR" && npm run build:h5)
}

cleanup_smoke() {
  if [[ -n "${SMOKE_PID:-}" ]] && kill -0 "$SMOKE_PID" >/dev/null 2>&1; then
    kill "$SMOKE_PID" >/dev/null 2>&1 || true
    wait "$SMOKE_PID" >/dev/null 2>&1 || true
  fi
}

run_smoke_test() {
  require_cmd curl
  mkdir -p "$TMP_DIR"

  local binary smoke_home log_file health_url ready
  binary="$TMP_DIR/pinhaoclaw-smoke"
  smoke_home="$(mktemp -d "$TMP_DIR/home.XXXXXX")"
  log_file="$TMP_DIR/smoke.log"
  health_url="http://127.0.0.1:${SMOKE_PORT}/health"
  ready=0

  log "building smoke binary"
  (cd "$ROOT_DIR" && CGO_ENABLED=0 go build -o "$binary" .)

  log "starting smoke instance on port ${SMOKE_PORT}"
  trap cleanup_smoke EXIT
  PINHAOCLAW_HOME="$smoke_home" \
  PINHAOCLAW_FRONTEND_DIR="$FRONTEND_DIR/dist/build/h5" \
  PINHAOCLAW_ADMIN_PASSWORD="smoke-admin" \
  "$binary" -H 127.0.0.1 -p "$SMOKE_PORT" >"$log_file" 2>&1 &
  SMOKE_PID=$!

  for _ in $(seq 1 15); do
    if curl -fsS "$health_url" >/dev/null 2>&1; then
      ready=1
      break
    fi
    sleep 1
  done

  if [[ "$ready" -ne 1 ]]; then
    cat "$log_file" >&2 || true
    fail "smoke test failed: /health not reachable"
  fi

  cleanup_smoke
  trap - EXIT
  log "smoke test passed"
}

main() {
  check_mode
  require_cmd go

  check_go_format
  run_go_tests

  if [[ "$MODE" == "commit" ]]; then
    log "commit checks passed"
    return
  fi

  require_cmd node
  require_cmd npm

  run_go_race_tests
  run_go_build
  run_frontend_build
  run_smoke_test

  log "${MODE} checks passed"
}

main "$@"