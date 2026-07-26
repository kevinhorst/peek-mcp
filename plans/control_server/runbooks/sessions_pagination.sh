#!/usr/bin/env bash
set -euo pipefail
BASE="${PEEK_CONTROL_URL:-http://127.0.0.1:4243}"
curl -s "$BASE/api/sessions?agent=claude&limit=2" | jq '{total, page: (.sessions | length), ids: [.sessions[].id]}'
curl -s "$BASE/api/sessions?agent=claude&limit=2&offset=2" | jq '{total, ids: [.sessions[].id]}'
curl -s "$BASE/fragments/sessions?agent=claude&offset=0" | grep -o 'evidence-table\|last-active-claude\|offset=50' | sort -u
curl -s -o /dev/null -w '%{http_code}\n' "$BASE/fragments/sessions"   # expect 400
