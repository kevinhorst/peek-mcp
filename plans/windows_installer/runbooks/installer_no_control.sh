#!/usr/bin/env bash
set -euo pipefail
SETUP_EXE="${1:?usage: installer_no_control.sh <path-to-peek-mcp-setup.exe>}"
"$SETUP_EXE" //VERYSILENT //SUPPRESSMSGBOXES //TASKS="claude"
grep -q -- '--control-port=0' "$USERPROFILE/.claude.json" && echo "control server off: OK"
