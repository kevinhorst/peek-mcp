package claude

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/kevinhorst/peek-mcp/session"
)

type Parser struct {
	store *session.Store
}

func NewParser(s *session.Store) *Parser {
	return &Parser{store: s}
}

func (p *Parser) ParseLine(line []byte) {
	entry := &Entry{}
	if err := json.Unmarshal(line, &entry); err != nil {
		return
	}
	if err := entry.Validate(); err != nil {
		return
	}

	if entry.IsSidechain {
		return
	}
	log.Printf("ClaudeParser: [%s], entry: %v", spew.Sdump(p), entry)

	switch entry.Type {
	case EntryTypeUser:
		p.handleUser(entry)
	case EntryTypeAssistant:
		p.handleAssistant(entry)
	}
}

func (p *Parser) handleUser(entry *Entry) {
	if entry.PromptId == "" {
		return
	}

	var message Message
	if err := json.Unmarshal(entry.Message, &message); err != nil {
		return
	}
	if err := message.Validate(); err != nil {
		return
	}

	var text string
	if err := json.Unmarshal(message.Content, &text); err != nil {
		return
	}

	if strings.TrimSpace(text) == "" {
		return
	}

	current := p.store.GetOrCreate(entry.SessionId, session.SourceClaude)
	p.updateMeta(current, entry, "")

	current.Turns.Push(&session.Turn{
		Role:      session.RoleUser,
		Text:      text,
		Timestamp: entry.Timestamp,
	})
}

func (p *Parser) handleAssistant(entry *Entry) {
	var message Message
	if err := json.Unmarshal(entry.Message, &message); err != nil {
		return
	}
	if err := message.Validate(); err != nil {
		return
	}

	text := extractTextBlocks(message.Content)

	if text == "" && entry.SessionId != "" {
		current := p.store.GetOrCreate(entry.SessionId, session.SourceClaude)
		p.updateMeta(current, entry, message.Model)
		return
	}

	var usage *session.Usage
	if message.Usage != nil {
		usage = &session.Usage{
			InputTokens:              message.Usage.InputTokens,
			OutputTokens:             message.Usage.OutputTokens,
			CacheCreationInputTokens: message.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     message.Usage.CacheReadInputTokens,
		}
	}

	current := p.store.GetOrCreate(entry.SessionId, session.SourceClaude)
	p.updateMeta(current, entry, message.Model)

	if usage != nil {
		current.TotalUsage.Add(usage)
	}

	current.Turns.Push(&session.Turn{
		Role:      session.RoleAssistant,
		Text:      text,
		Timestamp: entry.Timestamp,
		Model:     message.Model,
		RequestId: entry.RequestId,
		Usage:     usage,
	})
}

func (p *Parser) updateMeta(session *session.Session, entry *Entry, model string) {
	if !entry.Timestamp.IsZero() {
		session.LastActive = entry.Timestamp
	}

	if entry.CurrentWorkingDir != "" {
		session.CurrentWorkingDir = entry.CurrentWorkingDir
	}

	if entry.GitBranch != "" {
		session.GitBranch = entry.GitBranch
	}

	if model != "" {
		session.Model = model
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
