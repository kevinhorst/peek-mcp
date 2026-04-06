package claude

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/kevinhorst/peek-mcp/session"
)

type Parser struct {
	store            *session.Store
	lastRequestId    string
	pendingTurn      *session.Turn
	pendingSessionId session.Id
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

func (p *Parser) Flush() {
	p.flushPending()
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

	// No text content (thinking-only or tool_use-only) — still update meta
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

	// Same requestId means this is a continuation of the same logical response
	if entry.RequestId != "" && entry.RequestId == p.lastRequestId && p.pendingTurn != nil {
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

	p.lastRequestId = entry.RequestId
	p.pendingSessionId = entry.SessionId
	p.pendingTurn = &session.Turn{
		Role:      session.RoleAssistant,
		Text:      text,
		Timestamp: entry.Timestamp,
		Model:     message.Model,
		Usage:     usage,
	}

	current := p.store.GetOrCreate(entry.SessionId, session.SourceClaude)
	p.updateMeta(current, entry, message.Model)
}

func (p *Parser) flushPending() {
	if p.pendingTurn == nil || p.pendingSessionId == "" {
		return
	}

	current, ok := p.store.Get(p.pendingSessionId)
	if !ok {
		p.pendingTurn = nil
		p.pendingSessionId = ""
		p.lastRequestId = ""
		return
	}

	if p.pendingTurn.Usage != nil {
		current.TotalUsage.Add(p.pendingTurn.Usage)
	}

	current.Turns.Push(p.pendingTurn)
	p.pendingTurn = nil
	p.pendingSessionId = ""
	p.lastRequestId = ""
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
