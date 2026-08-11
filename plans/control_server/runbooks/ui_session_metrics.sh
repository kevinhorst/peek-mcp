#!/usr/bin/env bash
set -euo pipefail
BASE="${PEEK_CONTROL_URL:-http://127.0.0.1:4243}"
ID=$(curl -s "$BASE/api/sessions?agent=claude" | jq -r '.sessions[0].id')
curl -s "$BASE/fragments/sessions/$ID/usage" | grep -cE '<th>Session time</th>'
curl -s "$BASE/fragments/sessions/$ID/usage" | grep -cE '<th>Idle time</th>'
curl -s "$BASE/fragments/sessions/$ID/usage" | grep -cE '<th>Touched files</th>'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=subagents" | grep -cE '<th>Agent</th>|No subagents spawned'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=files" | grep -cE '<th>Path</th>|No touched files'
curl -s "$BASE/fragments/sessions/$ID/memory" | grep -cE 'md-body|<details|memory is not available|transcript path unknown|No memory directory'
curl -s "$BASE/api/sessions/$ID/memory" | jq -e 'has("index") or has("facts") or has("error")' >/dev/null && echo 1
CODEX=$(curl -s "$BASE/api/sessions?agent=codex" | jq -r '.sessions[0].id // empty')
[ -z "$CODEX" ] || curl -s "$BASE/fragments/sessions/$CODEX/memory" | grep -c 'memory is not available for codex sessions'
