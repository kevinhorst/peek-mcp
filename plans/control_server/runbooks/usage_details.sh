#!/usr/bin/env bash
set -euo pipefail
BASE="${PEEK_CONTROL_URL:-http://127.0.0.1:4243}"
ID=$(curl -s "$BASE/api/sessions?agent=claude" | jq -r '.sessions[0].id')
curl -s "$BASE/fragments/sessions/$ID/usage" | grep -c '<th>Model changes</th>'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=skills" | grep -cE '<th>Cost</th>|No skills invoked'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=plans" | grep -cE '<th>Phase</th>|No plan versions'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=models" | grep -cE '<th>From</th>|No model changes'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=cost" | grep -cE 'Estimate from embedded rates|No pricing'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=denials" | grep -cE '<th>Tool</th>|No permission denials'
curl -s -o /dev/null -w '%{http_code}\n' "$BASE/fragments/sessions/$ID/usage/skills"
