package watcher

import "github.com/kevinhorst/peek-mcp/session"

type parser interface {
	ParseLine(line []byte) *session.Turn
}
