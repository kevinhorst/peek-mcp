package models

type (
	SessionID     string
	SessionSource string
)

const (
	RoleUser                    = "user"
	RoleAssistant               = "assistant"
	RoleDeveloper               = "developer"
	SourceClaude  SessionSource = "claude"
	SourceCodex   SessionSource = "codex"
)
