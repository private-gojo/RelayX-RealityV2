#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

read -r -p "Cloud Run hostname (run.app): " RUN_APP
RUN_APP="${RUN_APP#https://}"
RUN_APP="${RUN_APP#wss://}"
RUN_APP="${RUN_APP#http://}"
RUN_APP="${RUN_APP%/}"
[[ -n "$RUN_APP" && "$RUN_APP" != */* ]] || {
  echo "[ERROR] Enter only the Cloud Run hostname." >&2
  exit 1
}

command -v go >/dev/null 2>&1 || { echo "[ERROR] Go is required." >&2; exit 1; }
XRAY_BIN="${XRAY_BIN:-xray}"
command -v "$XRAY_BIN" >/dev/null 2>&1 || {
  echo "[ERROR] Xray binary not found: $XRAY_BIN" >&2
  exit 1
}

export REMOTE_WS_URL="wss://$RUN_APP/reality"
export LOCAL_ADDR="${LOCAL_ADDR:-127.0.0.1:11000}"

mkdir -p bin
go build -trimpath -o bin/x-relay ./cmd/x-relay
./bin/x-relay client-config

./bin/x-relay client &
bridge_pid=$!
cleanup() {
  kill "$bridge_pid" 2>/dev/null || true
  wait "$bridge_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

"$XRAY_BIN" run -config client/xray-client.json
