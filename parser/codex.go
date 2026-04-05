package parser

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/kevinhorst/peek-mcp/models"
	"github.com/kevinhorst/peek-mcp/store"
)

const (
	codexMessageType    = "message"
	codexInputTextType  = "input_text"
	codexOutputTextType = "output_text"
)

type CodexParser struct {
	store     *store.Store
	sessionID string
	model     string
}

func NewCodexParser(s *store.Store) *CodexParser {
	return &CodexParser{store: s}
}

func (p *CodexParser) Flush() {}

func (p *CodexParser) ParseLine(line []byte) {
	var entry models.CodexEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return
	}
	if err := entry.Validate(); err != nil {
		return
	}

	switch entry.Type {
	case models.CodexEntryTypeSessionMeta:
		p.handleSessionMeta(entry.Payload, entry.Timestamp)
	case models.CodexEntryTypeTurnContext:
		p.handleTurnContext(entry.Payload)
	case models.CodexEntryTypeResponseItem:
		p.handleResponseItem(entry.Payload, entry.Timestamp)
	}
}

func (p *CodexParser) handleSessionMeta(payload json.RawMessage, ts time.Time) {
	var meta models.CodexSessionMeta
	if err := json.Unmarshal(payload, &meta); err != nil {
		return
	}
	if err := meta.Validate(); err != nil {
		return
	}

	p.sessionID = meta.ID

	session := p.store.GetOrCreate(meta.ID, string(models.SourceCodex))
	session.Meta.CWD = meta.CWD
	if !ts.IsZero() {
		session.Meta.LastActive = ts
	}
	if meta.Git != nil {
		session.Meta.GitBranch = meta.Git.CommitHash
	}
}

func (p *CodexParser) handleTurnContext(payload json.RawMessage) {
	var turnContext models.CodexTurnContext
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

func (p *CodexParser) handleResponseItem(payload json.RawMessage, ts time.Time) {
	if p.sessionID == "" {
		return
	}

	var item models.CodexResponseItem
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
	case models.RoleUser:
		p.handleUserMessage(&item, ts)
	case models.RoleAssistant:
		p.handleAssistantMessage(&item, ts)
	}
}

func (p *CodexParser) handleUserMessage(item *models.CodexResponseItem, ts time.Time) {
	text := p.extractText(item.Content, codexInputTextType)
	if text == "" {
		return
	}

	session := p.store.GetOrCreate(p.sessionID, string(models.SourceCodex))
	if !ts.IsZero() {
		session.Meta.LastActive = ts
	}

	session.Turns.Push(models.Turn{
		Role:      models.RoleUser,
		Text:      text,
		Timestamp: ts,
	})
}

func (p *CodexParser) handleAssistantMessage(item *models.CodexResponseItem, ts time.Time) {
	text := p.extractText(item.Content, codexOutputTextType)
	if text == "" {
		return
	}

	session := p.store.GetOrCreate(p.sessionID, string(models.SourceCodex))
	if !ts.IsZero() {
		session.Meta.LastActive = ts
	}
	if p.model != "" {
		session.Meta.Model = p.model
	}

	session.Turns.Push(models.Turn{
		Role:      models.RoleAssistant,
		Text:      text,
		Timestamp: ts,
		Model:     p.model,
	})
}

func (p *CodexParser) extractText(blocks []models.CodexContentBlock, targetType string) string {
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
