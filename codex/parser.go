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
	IndexFile  = "session_index.jsonl"

	// PlanFilePathProposedPlan marks Codex plans, which have no backing file.
	// The store never reads it: PlanContent is always set alongside.
	PlanFilePathProposedPlan = "codex:proposed_plan"

	codexMessageType    = "message"
	codexInputTextType  = "input_text"
	codexOutputTextType = "output_text"

	proposedPlanOpenTag  = "<proposed_plan>"
	proposedPlanCloseTag = "</proposed_plan>"
	maxPlanBytes         = 32 * 1024
)

const (
	execCommandTool         = "exec_command"
	functionCallOutputType  = "function_call_output"
	functionCallType        = "function_call"
	maxPendingCalls         = 64
	rejectedByUserMarker    = `Rejected("rejected by user")`
	sandboxRequireEscalated = "require_escalated"
)

type escalatedCall struct {
	cmd           string
	justification string
}

type execCommandArgs struct {
	Cmd                string `json:"cmd"`
	Justification      string `json:"justification"`
	SandboxPermissions string `json:"sandbox_permissions"`
}

type Parser struct {
	model            string
	pendingEscalated map[string]*escalatedCall
	sessionId        session.Id
	subagentActor    string
}

func NewParser() *Parser {
	return &Parser{pendingEscalated: make(map[string]*escalatedCall)}
}

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

	if meta.Source.IsSubagent() {
		return p.handleSubagentMeta(&meta, ts)
	}

	p.sessionId = meta.Id

	gitBranch := ""
	origin := &session.Origin{
		AgentNickname:  meta.Source.AgentNickname,
		CliVersion:     meta.CLIVersion,
		ForkedFromId:   meta.ForkedFromId,
		Originator:     meta.Originator,
		ParentThreadId: meta.Source.ParentThreadId,
		SourceKind:     meta.Source.Kind,
	}
	if meta.Git != nil {
		gitBranch = meta.Git.Branch
		origin.CommitHash = meta.Git.CommitHash
		origin.RepositoryUrl = meta.Git.RepositoryURL
	}

	return &session.Turn{
		Role:      session.RoleAssistant,
		Timestamp: ts,
		Meta: &session.Meta{
			SessionId: meta.Id,
			CWD:       meta.CWD,
			GitBranch: gitBranch,
			Origin:    origin,
		},
	}
}

func (p *Parser) handleSubagentMeta(meta *SessionMeta, ts time.Time) *session.Turn {
	if meta.Source.ParentThreadId == "" {
		slog.Debug("handleSubagentMeta: Dropping sub-agent rollout without parent", "id", meta.Id)
		return nil
	}

	p.sessionId = session.Id(meta.Source.ParentThreadId)
	p.subagentActor = meta.Source.AgentNickname
	if p.subagentActor == "" {
		p.subagentActor = string(meta.Id)
	}

	payload := &session.SubagentPayload{
		AgentId:   string(meta.Id),
		AgentType: meta.Source.AgentNickname,
	}
	event := &session.Event{
		Actor:     p.subagentActor,
		Kind:      session.EventKindSubagentSpawned,
		Subagent:  payload,
		Timestamp: ts,
	}

	turn := &session.Turn{
		Events: []*session.Event{event},
		Meta:   &session.Meta{SessionId: p.sessionId},
	}
	return turn
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
		return p.handleFunctionItem(&item, ts)
	}

	if p.subagentActor != "" {
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

func (p *Parser) handleFunctionItem(item *ResponseItem, ts time.Time) *session.Turn {
	switch item.Type {
	case functionCallType:
		p.rememberEscalatedCall(item)
		return nil
	case functionCallOutputType:
		return p.handleFunctionCallOutput(item, ts)
	}
	return nil
}

func (p *Parser) rememberEscalatedCall(item *ResponseItem) {
	if item.Name != execCommandTool || item.CallId == "" {
		return
	}

	var args execCommandArgs
	if err := json.Unmarshal([]byte(item.Arguments), &args); err != nil {
		slog.Debug("rememberEscalatedCall: unmarshal", "err", err)
		return
	}
	if args.SandboxPermissions != sandboxRequireEscalated {
		return
	}

	if len(p.pendingEscalated) >= maxPendingCalls {
		p.pendingEscalated = make(map[string]*escalatedCall)
	}
	p.pendingEscalated[item.CallId] = &escalatedCall{cmd: args.Cmd, justification: args.Justification}
}

func (p *Parser) handleFunctionCallOutput(item *ResponseItem, ts time.Time) *session.Turn {
	pending, ok := p.pendingEscalated[item.CallId]
	if !ok {
		return nil
	}

	delete(p.pendingEscalated, item.CallId)

	var output string
	if err := json.Unmarshal(item.Output, &output); err != nil {
		output = string(item.Output)
	}

	if !strings.Contains(output, rejectedByUserMarker) {
		return nil
	}

	payload := &session.PermissionPayload{
		Command:       pending.cmd,
		Justification: pending.justification,
		Tool:          execCommandTool,
	}
	event := &session.Event{
		Actor:      p.subagentActor,
		Kind:       session.EventKindPermissionDenied,
		Permission: payload,
		Timestamp:  ts,
	}

	turn := &session.Turn{
		Events: []*session.Event{event},
		Meta:   &session.Meta{SessionId: p.sessionId},
	}
	return turn
}

func (p *Parser) handleEventMessage(payload json.RawMessage, timestamp time.Time) *session.Turn {
	if p.sessionId == "" {
		return nil
	}
	if p.subagentActor != "" {
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

	turn := &session.Turn{
		Role:      session.RoleAssistant,
		Text:      text,
		Timestamp: ts,
		Meta: &session.Meta{
			SessionId: p.sessionId,
			Model:     p.model,
		},
	}

	if plan := extractProposedPlan(text); plan != "" {
		turn.PlanContent = plan
		turn.PlanFilePath = PlanFilePathProposedPlan
	}

	return turn
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

// extractProposedPlan returns the content of the last complete
// <proposed_plan> block; tags sit on their own lines per the plan-mode spec.
func extractProposedPlan(text string) string {
	lines := strings.Split(text, "\n")

	start := -1
	end := -1
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == proposedPlanOpenTag {
			start = index
			end = -1
			continue
		}

		isUnclosedBlock := start >= 0 && end < 0
		if trimmed == proposedPlanCloseTag && isUnclosedBlock {
			end = index
		}
	}
	if start < 0 || end < 0 {
		return ""
	}

	plan := strings.Join(lines[start+1:end], "\n")
	if len(plan) > maxPlanBytes {
		slog.Warn("extractProposedPlan: Plan exceeds size cap, truncating", "bytes", len(plan))
		plan = plan[:maxPlanBytes]
	}
	return plan
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
