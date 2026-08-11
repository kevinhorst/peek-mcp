#!/usr/bin/env bash
set -euo pipefail
BASE="${PEEK_CONTROL_URL:-http://127.0.0.1:4243}"
./peek-mcp.exe version
curl -s "$BASE/api/sessions?agent=claude" | jq '{total, first: .sessions[0].id}'
ID=$(curl -s "$BASE/api/sessions?agent=claude" | jq -r '.sessions[0].id')
curl -s "$BASE/api/sessions/$ID/turns" | jq '.turns | length'
GITDIR=$(git rev-parse --absolute-git-dir)
test -f "$GITDIR/peek-diff" && echo "peek-diff written: OK"
