package codex

import (
	"encoding/json"
	"strings"
	"time"

	session "github.com/kevinhorst/peek-mcp/session"
)

const (
	codexMessageType    = "message"
	codexInputTextType  = "input_text"
	codexOutputTextType = "output_text"
)

type Parser struct {
	store     *session.Store
	sessionId session.Id
	model     string
}

func NewParser(s *session.Store) *Parser {
	return &Parser{store: s}
}

func (p *Parser) Flush() {}

func (p *Parser) ParseLine(line []byte) {
	var entry Entry
	if err := json.Unmarshal(line, &entry); err != nil {
		return
	}
	if err := entry.Validate(); err != nil {
		return
	}

	switch entry.Type {
	case EntryTypeSessionMeta:
		p.handleSessionMeta(entry.Payload, entry.Timestamp)
	case EntryTypeTurnContext:
		p.handleTurnContext(entry.Payload)
	case EntryTypeResponseItem:
		p.handleResponseItem(entry.Payload, entry.Timestamp)
	case EntryTypeEventMessage:
		p.handleEventMessage(entry.Payload, entry.Timestamp)
	}
}

func (p *Parser) handleSessionMeta(payload json.RawMessage, ts time.Time) {
	var meta SessionMeta
	if err := json.Unmarshal(payload, &meta); err != nil {
		return
	}
	if err := meta.Validate(); err != nil {
		return
	}

	p.sessionId = meta.Id

	session := p.store.GetOrCreate(meta.Id, session.SourceCodex)
	session.Info.CWD = meta.CWD
	if !ts.IsZero() {
		session.Info.LastActive = ts
	}
	if meta.Git != nil {
		session.Info.GitBranch = meta.Git.CommitHash
	}
}

func (p *Parser) handleTurnContext(payload json.RawMessage) {
	var turnContext TurnContext
	if err := json.Unmarshal(payload, &turnContext); err != nil {
		return
	}
	if err := turnContext.Validate(); err != nil {
		return
	}
	if turnContext.Model != "" {
		p.model = turnContext.Model
	}
}

func (p *Parser) handleResponseItem(payload json.RawMessage, ts time.Time) {
	if p.sessionId == "" {
		return
	}

	var item ResponseItem
	if err := json.Unmarshal(payload, &item); err != nil {
		return
	}
	if err := item.Validate(); err != nil {
		return
	}

	if item.Type != codexMessageType {
		return
	}

	switch item.Role {
	case session.RoleUser:
		p.handleUserMessage(&item, ts)
	case session.RoleAssistant:
		p.handleAssistantMessage(&item, ts)
	}
}

func (p *Parser) handleEventMessage(payload json.RawMessage, timestamp time.Time) {
	if p.sessionId == "" {
		return
	}

	var eventMessage EventMessage
	if err := json.Unmarshal(payload, &eventMessage); err != nil {
		return
	}
	if err := eventMessage.Validate(); err != nil {
		return
	}
	if eventMessage.Type != EventTypeTokenCount || eventMessage.Info == nil || eventMessage.Info.TotalTokenUsage == nil {
		return
	}

	current := p.store.GetOrCreate(p.sessionId, session.SourceCodex)
	if !timestamp.IsZero() {
		current.Info.LastActive = timestamp
	}
	current.Info.TotalUsage = convertUsage(eventMessage.Info.TotalTokenUsage)
}

func (p *Parser) handleUserMessage(item *ResponseItem, ts time.Time) {
	text := p.extractText(item.Content, codexInputTextType)
	if text == "" {
		return
	}

	current := p.store.GetOrCreate(p.sessionId, session.SourceCodex)
	if !ts.IsZero() {
		current.Info.LastActive = ts
	}

	current.Turns.Push(&session.Turn{
		Role:      session.RoleUser,
		Text:      text,
		Timestamp: ts,
	})
}

func (p *Parser) handleAssistantMessage(item *ResponseItem, timestamp time.Time) {
	text := p.extractText(item.Content, codexOutputTextType)
	if text == "" {
		return
	}

	current := p.store.GetOrCreate(p.sessionId, session.SourceCodex)
	if !timestamp.IsZero() {
		current.Info.LastActive = timestamp
	}
	if p.model != "" {
		current.Info.Model = p.model
	}

	current.Turns.Push(&session.Turn{
		Role:      session.RoleAssistant,
		Text:      text,
		Timestamp: timestamp,
		Model:     p.model,
	})
}

func (p *Parser) extractText(blocks []ContentBlock, targetType string) string {
	var builder strings.Builder
	for _, block := range blocks {
		if block.Type == targetType && block.Text != "" {
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(block.Text)
		}
	}
	return builder.String()
}

func convertUsage(codexUsage *TokenUsage) session.Usage {
	if codexUsage == nil {
		return session.Usage{}
	}

	return session.Usage{
		InputTokens:           codexUsage.InputTokens,
		CachedInputTokens:     codexUsage.CachedInputTokens,
		OutputTokens:          codexUsage.OutputTokens,
		ReasoningOutputTokens: codexUsage.ReasoningOutputTokens,
		TotalTokens:           codexUsage.TotalTokens,
	}
}
