#!/usr/bin/env bash
set -euo pipefail
BIN="${PEEK_MCP_BIN:-./dist/peek-mcp}"
{
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"runbook","version":"0.0.1"}}}'
  echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  sleep "${PEEK_MCP_SCAN_WAIT:-3}"
  echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"session_get","arguments":{"agent":"claude","turns":false,"diff":false}}}'
} | "$BIN" start --transport=stdio 2>/dev/null \
  | jq -c 'select(.id == 2) | .result.structuredContent // .result.content[0].text | if type == "object" then keys - ["has_more"] else . end'
echo 'expect: ["plan"] (or [] when the latest session has no plan)'
