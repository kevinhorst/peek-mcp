#!/usr/bin/env bash
set -euo pipefail
BIN="${PEEK_MCP_BIN:-./dist/peek-mcp}"
{
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"runbook","version":"0.0.1"}}}'
  echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  echo '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
} | "$BIN" start --transport=stdio 2>/dev/null \
  | jq -c 'select(.id == 2) | [.result.tools[].name] | sort'
echo 'expect: ["session_get","session_list"]'
