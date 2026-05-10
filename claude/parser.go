package claude

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/kevinhorst/peek-mcp/session"
)

const ProjectsDir = "projects"

type Parser struct{}

func NewParser() *Parser { return &Parser{} }

func (p *Parser) ParseLine(line []byte) *session.Turn {
	entry := &Entry{}
	if err := json.Unmarshal(line, &entry); err != nil {
		log.Printf("Parser.ParseLine: %s", err.Error())
		return nil
	}
	if err := entry.Validate(); err != nil {
		log.Printf("Parser.ParseLine: %s", err.Error())
		return nil
	}

	if entry.IsSidechain {
		return nil
	}

	switch entry.Type {
	case EntryTypeUser, EntryTypeQueueOperation:
		return p.handleUser(entry)
	case EntryTypeAssistant:
		return p.handleAssistant(entry)
	case EntryTypeAttachment:
		return p.handleAttachment(entry)
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
		log.Printf("claude.Parser.handleUser: %s", err.Error())
		return nil
	}
	if err := message.Validate(); err != nil {
		log.Printf("claude.Parser.handleUser: %s", err.Error())
		return nil
	}

	text := extractTextBlocks(message.Content)

	if strings.TrimSpace(text) == "" {
		return nil
	}

	turn := &session.Turn{
		Role:      session.RoleUser,
		Text:      text,
		Timestamp: entry.Timestamp,
		Meta: &session.Meta{
			SessionId: entry.SessionId,
			CWD:       entry.CurrentWorkingDir,
			GitBranch: entry.GitBranch,
		},
	}

	err := turn.Validate()
	if err != nil {
		log.Printf("claude.Parser.handleUser: %s", err.Error())
		return nil
	}

	return turn
}

func (p *Parser) handleAssistant(entry *Entry) *session.Turn {
	var message Message
	if err := json.Unmarshal(entry.Message, &message); err != nil {
		log.Printf("claude.Parser.handleAssistant: %s", err.Error())
		return nil
	}
	if err := message.Validate(); err != nil {
		log.Printf("claude.Parser.handleAssistant: %s", err.Error())
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
	turn := &session.Turn{
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

	err := turn.Validate()
	if err != nil {
		log.Printf("claude.Parser.handleAssistant: %s", err.Error())
		return nil
	}

	return turn
}

func (p *Parser) handleAttachment(entry *Entry) *session.Turn {
	if entry.SessionId == "" || len(entry.AttachmentRaw) == 0 {
		return nil
	}

	var attachment Attachment
	if err := json.Unmarshal(entry.AttachmentRaw, &attachment); err != nil {
		log.Printf("claude.Parser.handleAttachment: %s", err.Error())
		return nil
	}

	if attachment.Type != AttachmentTypePlanMode && attachment.Type != AttachmentTypePlanFileReference {
		return nil
	}

	if attachment.PlanFilePath == "" {
		return nil
	}

	return &session.Turn{
		PlanFilePath: attachment.PlanFilePath,
		Meta: &session.Meta{
			SessionId: entry.SessionId,
		},
	}
}

func extractTextBlocks(raw json.RawMessage) string {
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var builder strings.Builder
		for _, block := range blocks {
			if block.Type != "text" || block.Text == "" {
				continue
			}
			builder.WriteString(block.Text + "\n")
		}
		return builder.String()
	}

	// user messages may carry content as a plain string rather than a block array
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}
