#!/usr/bin/env bash
set -euo pipefail
BASE="${PEEK_CONTROL_URL:-http://127.0.0.1:4243}"
ID=$(curl -s "$BASE/api/sessions?agent=claude" | jq -r '.sessions[0].id')
curl -s "$BASE/api/sessions/$ID/events" | jq '{counters, plan_revisions, total_tokens: .usage.total_tokens, event_count: (.events | length)}'
curl -s "$BASE/fragments/sessions/$ID/usage" | grep -c '<th>Input tokens</th>'
curl -s "$BASE/fragments/sessions/$ID/events" | grep -c 'card card-column\|No events yet'
curl -s -o /dev/null -w '%{http_code}\n' "$BASE/api/sessions/does-not-exist/events"
