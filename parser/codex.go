package parser

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/kevinhorst/peek-mcp/models"
	"github.com/kevinhorst/peek-mcp/store"
)

type CodexParser struct {
	store     *store.Store
	sessionID string
	model     string
}

func NewCodexParser(s *store.Store) *CodexParser {
	return &CodexParser{store: s}
}

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

	sess := p.store.GetOrCreate(meta.ID, "codex")
	sess.Meta.CWD = meta.CWD
	if !ts.IsZero() {
		sess.Meta.LastActive = ts
	}
	if meta.Git != nil {
		sess.Meta.GitBranch = meta.Git.CommitHash
	}
}

func (p *CodexParser) handleTurnContext(payload json.RawMessage) {
	var ctx models.CodexTurnContext
	if err := json.Unmarshal(payload, &ctx); err != nil {
		return
	}
	if err := ctx.Validate(); err != nil {
		return
	}
	if ctx.Model != "" {
		p.model = ctx.Model
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

	if item.Type != "message" {
		return
	}

	switch item.Role {
	case "user":
		p.handleUserMessage(&item, ts)
	case "assistant":
		p.handleAssistantMessage(&item, ts)
	}
}

func (p *CodexParser) handleUserMessage(item *models.CodexResponseItem, ts time.Time) {
	text := p.extractText(item.Content, "input_text")
	if text == "" {
		return
	}

	sess := p.store.GetOrCreate(p.sessionID, "codex")
	if !ts.IsZero() {
		sess.Meta.LastActive = ts
	}

	sess.Turns.Push(models.Turn{
		Role:      "user",
		Text:      text,
		Timestamp: ts,
	})
}

func (p *CodexParser) handleAssistantMessage(item *models.CodexResponseItem, ts time.Time) {
	text := p.extractText(item.Content, "output_text")
	if text == "" {
		return
	}

	sess := p.store.GetOrCreate(p.sessionID, "codex")
	if !ts.IsZero() {
		sess.Meta.LastActive = ts
	}
	if p.model != "" {
		sess.Meta.Model = p.model
	}

	sess.Turns.Push(models.Turn{
		Role:      "assistant",
		Text:      text,
		Timestamp: ts,
		Model:     p.model,
	})
}

func (p *CodexParser) extractText(blocks []models.CodexContentBlock, targetType string) string {
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == targetType && b.Text != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}
