package parser

import (
	"encoding/json"
	"strings"

	"github.com/kevinhorst/peek-mcp/models"
	"github.com/kevinhorst/peek-mcp/store"
)

const claudeTextContentType = "text"

type ClaudeParser struct {
	store          *store.Store
	lastRequestID  string
	pendingTurn    *models.Turn
	pendingSession string
}

func NewClaudeParser(s *store.Store) *ClaudeParser {
	return &ClaudeParser{store: s}
}

func (p *ClaudeParser) ParseLine(line []byte) {
	var entry models.ClaudeEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return
	}
	if err := entry.Validate(); err != nil {
		return
	}

	if entry.IsSidechain {
		return
	}

	switch entry.Type {
	case models.ClaudeEntryTypeUser:
		p.handleUser(&entry)
	case models.ClaudeEntryTypeAssistant:
		p.handleAssistant(&entry)
	}
}

func (p *ClaudeParser) Flush() {
	p.flushPending()
}

func (p *ClaudeParser) handleUser(entry *models.ClaudeEntry) {
	if entry.PromptID == "" {
		return
	}

	var message models.ClaudeMessage
	if err := json.Unmarshal(entry.Message, &message); err != nil {
		return
	}
	if err := message.Validate(); err != nil {
		return
	}

	// Genuine human prompt has string content, not array (tool_result)
	var text string
	if err := json.Unmarshal(message.Content, &text); err != nil {
		return
	}

	if strings.TrimSpace(text) == "" {
		return
	}

	// Flush any pending assistant turn before adding user turn
	p.flushPending()

	session := p.store.GetOrCreate(entry.SessionID, string(models.SourceClaude))
	p.updateMeta(session, entry, "")

	session.Turns.Push(models.Turn{
		Role:      models.RoleUser,
		Text:      text,
		Timestamp: entry.Timestamp,
	})
}

func (p *ClaudeParser) handleAssistant(entry *models.ClaudeEntry) {
	var message models.ClaudeMessage
	if err := json.Unmarshal(entry.Message, &message); err != nil {
		return
	}
	if err := message.Validate(); err != nil {
		return
	}

	text := extractTextBlocks(message.Content)
	if text == "" {
		// No text content (thinking-only or tool_use-only) — still update meta
		if entry.SessionID != "" {
			session := p.store.GetOrCreate(entry.SessionID, string(models.SourceClaude))
			p.updateMeta(session, entry, message.Model)
		}
		return
	}

	usage := convertUsage(message.Usage)

	// Same requestId means this is a continuation of the same logical response
	if entry.RequestID != "" && entry.RequestID == p.lastRequestID && p.pendingTurn != nil {
		p.pendingTurn.Text += text
		if usage != nil {
			p.pendingTurn.Usage = usage
		}
		if message.Model != "" {
			p.pendingTurn.Model = message.Model
		}
		return
	}

	// Different requestId — flush previous and start new pending turn
	p.flushPending()

	p.lastRequestID = entry.RequestID
	p.pendingSession = entry.SessionID
	p.pendingTurn = &models.Turn{
		Role:      models.RoleAssistant,
		Text:      text,
		Timestamp: entry.Timestamp,
		Model:     message.Model,
		Usage:     usage,
	}

	session := p.store.GetOrCreate(entry.SessionID, string(models.SourceClaude))
	p.updateMeta(session, entry, message.Model)
}

func (p *ClaudeParser) flushPending() {
	if p.pendingTurn == nil || p.pendingSession == "" {
		return
	}

	session, ok := p.store.Get(p.pendingSession)
	if !ok {
		p.pendingTurn = nil
		p.pendingSession = ""
		p.lastRequestID = ""
		return
	}

	if p.pendingTurn.Usage != nil {
		session.Meta.TotalUsage.Add(p.pendingTurn.Usage)
	}

	session.Turns.Push(*p.pendingTurn)
	p.pendingTurn = nil
	p.pendingSession = ""
	p.lastRequestID = ""
}

func (p *ClaudeParser) updateMeta(session *models.Session, entry *models.ClaudeEntry, model string) {
	if !entry.Timestamp.IsZero() {
		session.Meta.LastActive = entry.Timestamp
	}
	if entry.CWD != "" {
		session.Meta.CWD = entry.CWD
	}
	if entry.GitBranch != "" {
		session.Meta.GitBranch = entry.GitBranch
	}
	if model != "" {
		session.Meta.Model = model
	}
}

func extractTextBlocks(raw json.RawMessage) string {
	var blocks []models.ClaudeContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	var builder strings.Builder
	for _, block := range blocks {
		if block.Type == claudeTextContentType && block.Text != "" {
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(block.Text)
		}
	}
	return builder.String()
}

func convertUsage(claudeUsage *models.ClaudeUsage) *models.Usage {
	if claudeUsage == nil {
		return nil
	}
	return &models.Usage{
		InputTokens:              claudeUsage.InputTokens,
		OutputTokens:             claudeUsage.OutputTokens,
		CacheCreationInputTokens: claudeUsage.CacheCreationInputTokens,
		CacheReadInputTokens:     claudeUsage.CacheReadInputTokens,
	}
}
