#!/usr/bin/env bash
set -euo pipefail
SETUP_EXE="${1:?usage: installer_smoke.sh <path-to-peek-mcp-setup.exe>}"
"$SETUP_EXE" //VERYSILENT //SUPPRESSMSGBOXES //TASKS="claude,codex,controlserver,addtopath"
BIN="$LOCALAPPDATA/Programs/peek-mcp/peek-mcp.exe"
"$BIN" version
grep -q '"peek-mcp"' "$USERPROFILE/.claude.json" && echo "claude config: OK"
grep -q 'mcp_servers.peek-mcp' "$USERPROFILE/.codex/config.toml" && echo "codex config: OK"
! grep -q 'control-port=0' "$USERPROFILE/.claude.json" && echo "control server on: OK"
