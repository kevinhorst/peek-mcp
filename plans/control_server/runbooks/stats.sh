#!/usr/bin/env bash
set -euo pipefail
BASE="${PEEK_CONTROL_URL:-http://127.0.0.1:4243}"
curl -s "$BASE/api/stats" | jq '{pid, uptime, goroutines, sessions, state_disk_bytes, sse_clients, invocations, token_set: .config.token_set}'
curl -s "$BASE/stats" | grep -c 'href="/stats"'
curl -s "$BASE/fragments/stats" | grep -o '<th>PID</th>'
