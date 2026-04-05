package models

type (
	SessionID     string
	SessionSource string
)

const (
	RoleUser                    = "user"
	RoleAssistant               = "assistant"
	SourceClaude  SessionSource = "claude"
	SourceCodex   SessionSource = "codex"
)
