#!/usr/bin/env bash
set -euo pipefail
BASE="${PEEK_CONTROL_URL:-http://127.0.0.1:4243}"
ID=$(curl -s "$BASE/api/sessions?agent=claude" | jq -r '.sessions[0].id')
COUNT=$(curl -s "$BASE/api/sessions/$ID/turns" | jq '.turns | length')
echo "turns returned without n: $COUNT (expect min(20, --depth, available))"
curl -s "$BASE/fragments/sessions/$ID/turns" | grep -c 'card card-column'
