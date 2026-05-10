package codex

import (
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
)

const (
	SessionDir = "sessions"

	codexMessageType    = "message"
	codexInputTextType  = "input_text"
	codexOutputTextType = "output_text"
)

type Parser struct {
	sessionId session.Id
	model     string
}

func NewParser() *Parser { return &Parser{} }

func (p *Parser) ParseLine(line []byte) *session.Turn {
	var entry Entry
	if err := json.Unmarshal(line, &entry); err != nil {
		slog.Debug("ParseLine: unmarshal", "err", err)
		return nil
	}

	if err := entry.Validate(); err != nil {
		slog.Debug("ParseLine: validate", "err", err)
		return nil
	}

	switch entry.Type {
	case EntryTypeSessionMeta:
		return p.handleSessionMeta(entry.Payload, entry.Timestamp)
	case EntryTypeTurnContext:
		p.handleTurnContext(entry.Payload)
		return nil
	case EntryTypeResponseItem:
		return p.handleResponseItem(entry.Payload, entry.Timestamp)
	case EntryTypeEventMessage:
		return p.handleEventMessage(entry.Payload, entry.Timestamp)
	default:
		return nil
	}
}

func (p *Parser) handleSessionMeta(payload json.RawMessage, ts time.Time) *session.Turn {
	var meta SessionMeta
	if err := json.Unmarshal(payload, &meta); err != nil {
		slog.Debug("handleSessionMeta: unmarshal", "err", err)
		return nil
	}
	if err := meta.Validate(); err != nil {
		slog.Debug("handleSessionMeta: validate", "err", err)
		return nil
	}

	p.sessionId = meta.Id

	gitBranch := ""
	if meta.Git != nil {
		gitBranch = meta.Git.CommitHash
	}

	return &session.Turn{
		Role:      session.RoleAssistant,
		Timestamp: ts,
		Meta: &session.Meta{
			SessionId: meta.Id,
			CWD:       meta.CWD,
			GitBranch: gitBranch,
		},
	}
}

func (p *Parser) handleTurnContext(payload json.RawMessage) {
	var turnContext TurnContext
	if err := json.Unmarshal(payload, &turnContext); err != nil {
		slog.Debug("handleTurnContext: unmarshal", "err", err)
		return
	}
	if err := turnContext.Validate(); err != nil {
		slog.Debug("handleTurnContext: validate", "err", err)
		return
	}
	if turnContext.Model != "" {
		p.model = turnContext.Model
	}
}

func (p *Parser) handleResponseItem(payload json.RawMessage, ts time.Time) *session.Turn {
	if p.sessionId == "" {
		return nil
	}

	var item ResponseItem
	if err := json.Unmarshal(payload, &item); err != nil {
		slog.Debug("handleResponseItem: unmarshal", "err", err)
		return nil
	}
	if err := item.Validate(); err != nil {
		slog.Debug("handleResponseItem: validate", "err", err)
		return nil
	}

	if item.Type != codexMessageType {
		return nil
	}

	switch item.Role {
	case session.RoleUser:
		return p.handleUserMessage(&item, ts)
	case session.RoleAssistant:
		return p.handleAssistantMessage(&item, ts)
	default:
		return nil
	}
}

func (p *Parser) handleEventMessage(payload json.RawMessage, timestamp time.Time) *session.Turn {
	if p.sessionId == "" {
		return nil
	}

	var eventMessage EventMessage
	if err := json.Unmarshal(payload, &eventMessage); err != nil {
		slog.Debug("handleEventMessage: unmarshal", "err", err)
		return nil
	}
	if err := eventMessage.Validate(); err != nil {
		slog.Debug("handleEventMessage: validate", "err", err)
		return nil
	}
	if eventMessage.Type != EventTypeTokenCount || eventMessage.Info == nil || eventMessage.Info.TotalTokenUsage == nil {
		return nil
	}

	usage := convertUsage(eventMessage.Info.TotalTokenUsage)
	return &session.Turn{
		Timestamp: timestamp,
		Usage:     &usage,
		Meta: &session.Meta{
			SessionId: p.sessionId,
		},
	}
}

func (p *Parser) handleUserMessage(item *ResponseItem, ts time.Time) *session.Turn {
	text := p.extractText(item.Content, codexInputTextType)
	if text == "" {
		slog.Debug("handleUserMessage: no input text found")
		return nil
	}

	return &session.Turn{
		Role:      session.RoleUser,
		Text:      text,
		Timestamp: ts,
		Meta: &session.Meta{
			SessionId: p.sessionId,
		},
	}
}

func (p *Parser) handleAssistantMessage(item *ResponseItem, ts time.Time) *session.Turn {
	text := p.extractText(item.Content, codexOutputTextType)
	if text == "" {
		slog.Debug("handleAssistantMessage: no output text found")
		return nil
	}

	return &session.Turn{
		Role:      session.RoleAssistant,
		Text:      text,
		Timestamp: ts,
		Meta: &session.Meta{
			SessionId: p.sessionId,
			Model:     p.model,
		},
	}
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
