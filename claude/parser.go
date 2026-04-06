package claude

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/kevinhorst/peek-mcp/models"
	"github.com/kevinhorst/peek-mcp/store"
)

const claudeTextContentType = "text"

type Parser struct {
	store          *store.Store
	lastRequestID  string
	pendingTurn    *models.Turn
	pendingSession string
}

func NewParser(s *store.Store) *Parser {
	return &Parser{store: s}
}

func (p *Parser) ParseLine(line []byte) {
	var entry Entry
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
		p.handleUser(&entry)
	case EntryTypeAssistant:
		p.handleAssistant(&entry)
	}
}

func (p *Parser) Flush() {
	p.flushPending()
}

func (p *Parser) handleUser(entry *Entry) {
	if entry.PromptID == "" {
		return
	}

	var message Message
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

	session.Turns.Push(&models.Turn{
		Role:      models.RoleUser,
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
	if text == "" {
		// No text content (thinking-only or tool_use-only) — still update meta
		if entry.SessionID != "" {
			session := p.store.GetOrCreate(entry.SessionID, string(models.SourceClaude))
			p.updateMeta(session, entry, message.Model)
		}
		return
	}

	var usage *models.Usage
	if message.Usage != nil {
		usage = &models.Usage{
			InputTokens:              message.Usage.InputTokens,
			OutputTokens:             message.Usage.OutputTokens,
			CacheCreationInputTokens: message.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     message.Usage.CacheReadInputTokens,
		}
	}

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

func (p *Parser) flushPending() {
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
		session.Info.TotalUsage.Add(p.pendingTurn.Usage)
	}

	session.Turns.Push(p.pendingTurn)
	p.pendingTurn = nil
	p.pendingSession = ""
	p.lastRequestID = ""
}

func (p *Parser) updateMeta(session *models.Session, entry *Entry, model string) {
	if !entry.Timestamp.IsZero() {
		session.Info.LastActive = entry.Timestamp
	}
	if entry.CWD != "" {
		session.Info.CWD = entry.CWD
	}
	if entry.GitBranch != "" {
		session.Info.GitBranch = entry.GitBranch
	}
	if model != "" {
		session.Info.Model = model
	}
}

func extractTextBlocks(raw json.RawMessage) string {
	var blocks []ContentBlock
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
