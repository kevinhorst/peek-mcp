package watcher

import "github.com/kevinhorst/peek-mcp/session"

type Parser interface {
	ParseLine(line []byte) *session.Turn
}
