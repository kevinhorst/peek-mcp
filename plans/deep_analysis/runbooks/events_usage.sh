#!/bin/sh
curl -s http://127.0.0.1:4242/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "session_events",
    "arguments": {
      "agent": "claude",
      "json": true
    }
  }
}'
