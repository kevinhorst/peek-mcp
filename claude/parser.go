package claude

import (
	"encoding/json"
	"strings"

	"github.com/kevinhorst/peek-mcp/session"
)

const ProjectsDir = "projects"

type Parser struct{}

func NewParser() *Parser { return &Parser{} }

func (p *Parser) ParseLine(line []byte) *session.Turn {
	entry := &Entry{}
	if err := json.Unmarshal(line, &entry); err != nil {
		return nil
	}
	if err := entry.Validate(); err != nil {
		return nil
	}

	if entry.IsSidechain {
		return nil
	}

	switch entry.Type {
	case EntryTypeUser:
		return p.handleUser(entry)
	case EntryTypeAssistant:
		return p.handleAssistant(entry)
	default:
		return nil
	}
}

func (p *Parser) handleUser(entry *Entry) *session.Turn {
	if entry.PromptId == "" {
		return nil
	}

	var message Message
	if err := json.Unmarshal(entry.Message, &message); err != nil {
		return nil
	}
	if err := message.Validate(); err != nil {
		return nil
	}

	var text string
	if err := json.Unmarshal(message.Content, &text); err != nil {
		return nil
	}

	if strings.TrimSpace(text) == "" {
		return nil
	}

	return &session.Turn{
		Role:      session.RoleUser,
		Text:      text,
		Timestamp: entry.Timestamp,
		Meta: &session.Meta{
			SessionId: entry.SessionId,
			CWD:       entry.CurrentWorkingDir,
			GitBranch: entry.GitBranch,
		},
	}
}

func (p *Parser) handleAssistant(entry *Entry) *session.Turn {
	var message Message
	if err := json.Unmarshal(entry.Message, &message); err != nil {
		return nil
	}
	if err := message.Validate(); err != nil {
		return nil
	}

	text := extractTextBlocks(message.Content)

	var usage *session.Usage
	if message.Usage != nil {
		usage = &session.Usage{
			InputTokens:              message.Usage.InputTokens,
			OutputTokens:             message.Usage.OutputTokens,
			CacheCreationInputTokens: message.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     message.Usage.CacheReadInputTokens,
		}
	}

	return &session.Turn{
		Role:      session.RoleAssistant,
		Text:      text,
		Timestamp: entry.Timestamp,
		RequestId: entry.RequestId,
		Usage:     usage,
		Meta: &session.Meta{
			SessionId: entry.SessionId,
			CWD:       entry.CurrentWorkingDir,
			GitBranch: entry.GitBranch,
			Model:     message.Model,
		},
	}
}

func extractTextBlocks(raw json.RawMessage) string {
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	var builder strings.Builder
	for _, block := range blocks {
		if block.Type != "text" || block.Text == "" {
			continue
		}

		builder.WriteString(block.Text + "\n")
	}

	return builder.String()
}
