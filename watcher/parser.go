package watcher

import "github.com/kevinhorst/peek-mcp/session"

type Parser interface {
	ParseLine(line []byte) *session.Turn
	// Restore loads what State returned into a fresh parser.
	Restore(state []byte) error
	// State returns what the parser carries from one line to the next.
	State() ([]byte, error)
}
