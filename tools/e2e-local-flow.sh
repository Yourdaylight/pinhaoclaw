#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${PINHAOCLAW_E2E_PORT:-19100}"
TMP_DIR="${PINHAOCLAW_E2E_TMPDIR:-$(mktemp -d "$ROOT_DIR/.tmp/e2e-local-flow.XXXXXX")}"
DATA_DIR="$TMP_DIR/data"
NODE_HOME="$TMP_DIR/local-node"
BIN_PATH="$TMP_DIR/pinhaoclaw-e2e"
LOG_PATH="$TMP_DIR/server.log"
BASE_URL="http://127.0.0.1:${PORT}"
FRONTEND_DIR="$ROOT_DIR/pinhaoclaw-frontend/dist/build/h5"
PICOCLAW_BIN="${PINHAOCLAW_PICOCLAW_BIN:-$ROOT_DIR/../picoclaw/build/picoclaw}"

cleanup() {
  if [[ -n "${SERVER_PID:-}" ]] && kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing command: $1" >&2
    exit 1
  }
}

json_field() {
  local json_input="$1"
  local expr="$2"
  python3 -c 'import json,sys
data=json.loads(sys.argv[1])
expr=sys.argv[2].split(".")
cur=data
for part in expr:
    cur=cur[part]
print(cur)' "$json_input" "$expr"
}

wait_for_health() {
  for _ in $(seq 1 20); do
    if curl -fsS "$BASE_URL/health" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "server did not become healthy" >&2
  cat "$LOG_PATH" >&2 || true
  exit 1
}

require_cmd curl
require_cmd python3
require_cmd go

mkdir -p "$ROOT_DIR/.tmp" "$DATA_DIR" "$NODE_HOME"

if [[ ! -f "$FRONTEND_DIR/index.html" ]]; then
  echo "frontend build missing at $FRONTEND_DIR; run npm run build:h5 first" >&2
  exit 1
fi

if [[ ! -x "$PICOCLAW_BIN" ]]; then
  echo "picoclaw binary missing or not executable: $PICOCLAW_BIN" >&2
  echo "set PINHAOCLAW_PICOCLAW_BIN to a valid picoclaw binary path" >&2
  exit 1
fi

echo "[e2e] building backend binary"
(cd "$ROOT_DIR" && go build -o "$BIN_PATH" .)

echo "[e2e] starting temporary server on $BASE_URL"
PINHAOCLAW_HOME="$DATA_DIR" \
PINHAOCLAW_FRONTEND_DIR="$FRONTEND_DIR" \
PINHAOCLAW_AUTH_MODE="invite" \
PINHAOCLAW_ADMIN_PASSWORD="local-e2e-admin" \
PINHAOCLAW_PUBLIC_ORIGIN="$BASE_URL" \
PINHAOCLAW_PICOCLAW_BIN="$PICOCLAW_BIN" \
"$BIN_PATH" -H 127.0.0.1 -p "$PORT" >"$LOG_PATH" 2>&1 &
SERVER_PID=$!

wait_for_health

echo "[e2e] admin login"
ADMIN_LOGIN_JSON="$(curl -fsS -X POST "$BASE_URL/api/admin/login" -H 'Content-Type: application/json' -d '{"password":"local-e2e-admin"}')"
ADMIN_TOKEN="$(json_field "$ADMIN_LOGIN_JSON" token)"

echo "[e2e] creating local node"
ADD_NODE_JSON="$(curl -fsS -X POST "$BASE_URL/api/admin/nodes" \
  -H 'Content-Type: application/json' \
  -H "X-Admin-Token: $ADMIN_TOKEN" \
  -d "{\"type\":\"local\",\"name\":\"本地售卖节点\",\"host\":\"local\",\"remote_home\":\"$NODE_HOME\",\"region\":\"华南\"}")"
NODE_ID="$(json_field "$ADD_NODE_JSON" node.id)"

echo "[e2e] testing local node"
curl -fsS -X POST "$BASE_URL/api/admin/nodes/$NODE_ID/test" -H "X-Admin-Token: $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{}' >/dev/null

echo "[e2e] creating invite"
INVITE_JSON="$(curl -fsS -X POST "$BASE_URL/api/admin/invites" -H "X-Admin-Token: $ADMIN_TOKEN" -H 'Content-Type: application/json' -d '{}')"
INVITE_CODE="$(json_field "$INVITE_JSON" code)"

echo "[e2e] user login with invite"
USER_LOGIN_JSON="$(curl -fsS -X POST "$BASE_URL/api/auth/login" -H 'Content-Type: application/json' -d "{\"invite_code\":\"$INVITE_CODE\",\"name\":\"本地验证用户\"}")"
USER_TOKEN="$(json_field "$USER_LOGIN_JSON" token)"

echo "[e2e] creating lobster"
CREATE_JSON="$(curl -fsS -X POST "$BASE_URL/api/lobsters" -H 'Content-Type: application/json' -H "X-User-Token: $USER_TOKEN" -d '{"name":"本地龙虾","region":"华南"}')"
LOBSTER_ID="$(json_field "$CREATE_JSON" lobster.id)"

echo "[e2e] starting lobster"
curl -fsS -X POST "$BASE_URL/api/lobsters/$LOBSTER_ID/start" -H "X-User-Token: $USER_TOKEN" -H 'Content-Type: application/json' -d '{}' >/dev/null

echo "[e2e] listing user lobsters"
LIST_JSON="$(curl -fsS "$BASE_URL/api/lobsters" -H "X-User-Token: $USER_TOKEN")"

echo "[e2e] stopping and deleting lobster"
curl -fsS -X POST "$BASE_URL/api/lobsters/$LOBSTER_ID/stop" -H "X-User-Token: $USER_TOKEN" -H 'Content-Type: application/json' -d '{}' >/dev/null
curl -fsS -X DELETE "$BASE_URL/api/lobsters/$LOBSTER_ID" -H "X-User-Token: $USER_TOKEN" >/dev/null

echo "[e2e] flow completed"
echo "node_id=$NODE_ID"
echo "invite_code=$INVITE_CODE"
echo "lobster_id=$LOBSTER_ID"
echo "lobsters_response=$LIST_JSON"