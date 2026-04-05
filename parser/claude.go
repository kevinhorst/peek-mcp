package parser

import (
	"encoding/json"
	"strings"

	"github.com/kevinhorst/peek-mcp/models"
	"github.com/kevinhorst/peek-mcp/store"
)

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

	var msg models.ClaudeMessage
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return
	}
	if err := msg.Validate(); err != nil {
		return
	}

	// Genuine human prompt has string content, not array (tool_result)
	var text string
	if err := json.Unmarshal(msg.Content, &text); err != nil {
		return
	}

	if strings.TrimSpace(text) == "" {
		return
	}

	// Flush any pending assistant turn before adding user turn
	p.flushPending()

	sess := p.store.GetOrCreate(entry.SessionID, "claude")
	p.updateMeta(sess, entry, "")

	sess.Turns.Push(models.Turn{
		Role:      "user",
		Text:      text,
		Timestamp: entry.Timestamp,
	})
}

func (p *ClaudeParser) handleAssistant(entry *models.ClaudeEntry) {
	var msg models.ClaudeMessage
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return
	}
	if err := msg.Validate(); err != nil {
		return
	}

	text := extractTextBlocks(msg.Content)
	if text == "" {
		// No text content (thinking-only or tool_use-only) — still update meta
		if entry.SessionID != "" {
			sess := p.store.GetOrCreate(entry.SessionID, "claude")
			p.updateMeta(sess, entry, msg.Model)
		}
		return
	}

	usage := convertUsage(msg.Usage)

	// Same requestId means this is a continuation of the same logical response
	if entry.RequestID != "" && entry.RequestID == p.lastRequestID && p.pendingTurn != nil {
		p.pendingTurn.Text += text
		if usage != nil {
			p.pendingTurn.Usage = usage
		}
		if msg.Model != "" {
			p.pendingTurn.Model = msg.Model
		}
		return
	}

	// Different requestId — flush previous and start new pending turn
	p.flushPending()

	p.lastRequestID = entry.RequestID
	p.pendingSession = entry.SessionID
	p.pendingTurn = &models.Turn{
		Role:      "assistant",
		Text:      text,
		Timestamp: entry.Timestamp,
		Model:     msg.Model,
		Usage:     usage,
	}

	sess := p.store.GetOrCreate(entry.SessionID, "claude")
	p.updateMeta(sess, entry, msg.Model)
}

func (p *ClaudeParser) flushPending() {
	if p.pendingTurn == nil || p.pendingSession == "" {
		return
	}

	sess, ok := p.store.Get(p.pendingSession)
	if !ok {
		p.pendingTurn = nil
		p.pendingSession = ""
		p.lastRequestID = ""
		return
	}

	if p.pendingTurn.Usage != nil {
		sess.Meta.TotalUsage.Add(p.pendingTurn.Usage)
	}

	sess.Turns.Push(*p.pendingTurn)
	p.pendingTurn = nil
	p.pendingSession = ""
	p.lastRequestID = ""
}

func (p *ClaudeParser) updateMeta(sess *models.Session, entry *models.ClaudeEntry, model string) {
	if !entry.Timestamp.IsZero() {
		sess.Meta.LastActive = entry.Timestamp
	}
	if entry.CWD != "" {
		sess.Meta.CWD = entry.CWD
	}
	if entry.GitBranch != "" {
		sess.Meta.GitBranch = entry.GitBranch
	}
	if model != "" {
		sess.Meta.Model = model
	}
}

func extractTextBlocks(raw json.RawMessage) string {
	var blocks []models.ClaudeContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

func convertUsage(cu *models.ClaudeUsage) *models.Usage {
	if cu == nil {
		return nil
	}
	return &models.Usage{
		InputTokens:              cu.InputTokens,
		OutputTokens:             cu.OutputTokens,
		CacheCreationInputTokens: cu.CacheCreationInputTokens,
		CacheReadInputTokens:     cu.CacheReadInputTokens,
	}
}
