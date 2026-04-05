package models

type SessionID string

type SessionSource string

const (
	SourceClaude SessionSource = "claude"
	SourceCodex  SessionSource = "codex"
)
