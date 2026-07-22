package claude

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kevinhorst/peek-mcp/session"
)

const ProjectsDir = "projects"

const (
	toolNameAgent           = "Agent"
	toolNameAskUserQuestion = "AskUserQuestion"
	toolNameExitPlanMode    = "ExitPlanMode"
	toolNameSkill           = "Skill"

	contentTypeToolResult = "tool_result"
	contentTypeToolUse    = "tool_use"

	approvalPrefix        = "User has approved your plan"
	denialPrefix          = "The user doesn't want to proceed with this tool use."
	persistedOutputMarker = "<persisted-output>"
	toolResultsDir        = "tool-results"

	commandNameOpenTag  = "<command-name>"
	commandNameCloseTag = "</command-name>"
	commandArgsOpenTag  = "<command-args>"
	commandArgsCloseTag = "</command-args>"

	maxApprovedPlanBytes   = 64 * 1024
	maxPendingTools        = 64
	maxPersistedReadBytes  = 256 * 1024
	maxSubagentResultBytes = 32 * 1024
)

type Parser struct {
	pendingTools map[string]*pendingToolUse
}

func NewParser() *Parser {
	return &Parser{pendingTools: make(map[string]*pendingToolUse)}
}

type pendingToolUse struct {
	input json.RawMessage
	name  string
}

type askUserQuestion struct {
	Question string `json:"question"`
}

type askUserQuestionInput struct {
	Questions []askUserQuestion `json:"questions"`
}

type skillInput struct {
	Args  string `json:"args"`
	Skill string `json:"skill"`
}

func (p *Parser) ParseLine(line []byte) *session.Turn {
	entry := &Entry{}
	if err := json.Unmarshal(line, &entry); err != nil {
		slog.Debug("Parser.ParseLine: unmarshal", "err", err)
		return nil
	}
	if err := entry.Validate(); err != nil {
		slog.Debug("Parser.ParseLine: validate", "err", err)
		return nil
	}

	if entry.IsSidechain {
		return p.handleSidechain(entry)
	}

	switch entry.Type {
	case EntryTypeUser, EntryTypeQueueOperation:
		return p.handleUser(entry)
	case EntryTypeAssistant:
		return p.handleAssistant(entry)
	case EntryTypeAttachment:
		return p.handleAttachment(entry)
	case EntryTypeCustomTitle:
		return p.handleCustomTitle(entry)
	default:
		return nil
	}
}

func (p *Parser) handleUser(entry *Entry) *session.Turn {
	var message Message
	if err := json.Unmarshal(entry.Message, &message); err != nil {
		slog.Debug("handleUser: unmarshal", "err", err)
		return nil
	}
	if err := message.Validate(); err != nil {
		slog.Debug("handleUser: validate", "err", err)
		return nil
	}

	events := p.eventsFromUserContent(entry, &message)

	text := extractTextBlocks(message.Content)
	if event := slashCommandEvent(entry, text); event != nil {
		events = append(events, event)
	}

	isPrompt := entry.PromptId != "" && strings.TrimSpace(text) != ""
	if !isPrompt {
		return eventTurn(entry, events)
	}

	turn := &session.Turn{
		Events:    events,
		Role:      session.RoleUser,
		Text:      text,
		Timestamp: entry.Timestamp,
		Meta: &session.Meta{
			SessionId: entry.SessionId,
			CWD:       entry.CurrentWorkingDir,
			GitBranch: entry.GitBranch,
			Origin:    originFromEntry(entry),
		},
	}

	err := turn.Validate()
	if err != nil {
		slog.Debug("handleUser: turn validate", "err", err)
		return nil
	}

	return turn
}

func (p *Parser) handleAssistant(entry *Entry) *session.Turn {
	var message Message
	if err := json.Unmarshal(entry.Message, &message); err != nil {
		slog.Debug("handleAssistant: unmarshal", "err", err)
		return nil
	}
	if err := message.Validate(); err != nil {
		slog.Debug("handleAssistant: validate", "err", err)
		return nil
	}

	text := extractTextBlocks(message.Content)
	events := p.eventsFromAssistantContent(entry, &message)

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
		Events:    events,
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
			Origin:    originFromEntry(entry),
		},
	}

	err := turn.Validate()
	if err != nil {
		slog.Debug("handleAssistant: turn validate", "err", err)
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
		slog.Debug("handleAttachment: unmarshal", "err", err)
		return nil
	}

	if !isPlanAttachment(attachment.Type) {
		return nil
	}

	events := planModeEvents(entry, attachment.Type)

	if attachment.PlanFilePath == "" {
		return eventTurn(entry, events)
	}

	return &session.Turn{
		Events:       events,
		PlanFilePath: attachment.PlanFilePath,
		PlanContent:  attachment.PlanContent,
		Meta: &session.Meta{
			SessionId: entry.SessionId,
			CWD:       entry.CurrentWorkingDir,
		},
	}
}

func (p *Parser) handleSidechain(entry *Entry) *session.Turn {
	var message Message
	if err := json.Unmarshal(entry.Message, &message); err != nil {
		return nil
	}

	var events []*session.Event
	switch entry.Type {
	case EntryTypeUser:
		events = p.eventsFromUserContent(entry, &message)
	case EntryTypeAssistant:
		events = p.eventsFromAssistantContent(entry, &message)
	}

	return eventTurn(entry, events)
}

func (p *Parser) eventsFromAssistantContent(entry *Entry, message *Message) []*session.Event {
	blocks := contentBlocks(message.Content)

	events := make([]*session.Event, 0)
	for index := range blocks {
		block := &blocks[index]
		if block.Type != contentTypeToolUse {
			continue
		}

		p.rememberToolUse(block)

		if block.Name == toolNameSkill {
			events = append(events, skillEvent(entry, block))
		}
	}

	if len(events) == 0 {
		return nil
	}
	return events
}

func (p *Parser) eventsFromUserContent(entry *Entry, message *Message) []*session.Event {
	blocks := contentBlocks(message.Content)

	events := make([]*session.Event, 0)
	for index := range blocks {
		block := &blocks[index]
		if block.Type != contentTypeToolResult {
			continue
		}

		pending, ok := p.pendingTools[block.ToolUseId]
		if !ok {
			continue
		}
		delete(p.pendingTools, block.ToolUseId)

		event := toolResultEvent(entry, block, pending)
		if event != nil {
			events = append(events, event)
		}
	}

	if len(events) == 0 {
		return nil
	}
	return events
}

func (p *Parser) rememberToolUse(block *ContentBlock) {
	if block.Id == "" {
		return
	}

	if len(p.pendingTools) >= maxPendingTools {
		p.pendingTools = make(map[string]*pendingToolUse)
	}
	p.pendingTools[block.Id] = &pendingToolUse{input: block.Input, name: block.Name}
}

func (p *Parser) handleCustomTitle(entry *Entry) *session.Turn {
	if entry.CustomTitle == "" {
		return nil
	}

	return &session.Turn{
		CustomTitle: entry.CustomTitle,
		Meta: &session.Meta{
			SessionId: entry.SessionId,
		},
		TitleSource: session.TitleSourceCustom,
	}
}

func contentBlocks(raw json.RawMessage) []ContentBlock {
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	return blocks
}

func eventTurn(entry *Entry, events []*session.Event) *session.Turn {
	if len(events) == 0 {
		return nil
	}

	turn := &session.Turn{
		Events: events,
		Meta: &session.Meta{
			SessionId: entry.SessionId,
			CWD:       entry.CurrentWorkingDir,
		},
	}
	return turn
}

// The ExitPlanMode rejection reuses the generic denial text, so the pending
// tool name — not the message — decides rejected-vs-denied (plan D12).
func toolResultEvent(entry *Entry, block *ContentBlock, pending *pendingToolUse) *session.Event {
	var text string
	if err := json.Unmarshal(block.Content, &text); err != nil {
		text = extractTextBlocks(block.Content)
	}

	isDenied := block.IsError && strings.HasPrefix(text, denialPrefix)

	switch pending.name {
	case toolNameExitPlanMode:
		return planVerdictEvent(entry, block, text)
	case toolNameAgent:
		return subagentResultEvent(entry, block, text, isDenied)
	case toolNameAskUserQuestion:
		return userAnswerEvent(entry, block, pending, text, isDenied)
	default:
		if !isDenied {
			return nil
		}
		return permissionDeniedEvent(entry, pending.name)
	}
}

func planVerdictEvent(entry *Entry, block *ContentBlock, text string) *session.Event {
	if block.IsError {
		return &session.Event{
			Actor:     entry.AgentId,
			Kind:      session.EventKindPlanRejected,
			Timestamp: entry.Timestamp,
		}
	}

	content := resolvePersistedOutput(text, entry.SessionId, block.ToolUseId)
	if !strings.Contains(content, approvalPrefix) {
		return nil
	}

	if len(content) > maxApprovedPlanBytes {
		content = content[:maxApprovedPlanBytes] + "\n[peek: approved plan truncated at 64 KB]\n"
	}
	payload := &session.PlanPayload{Content: content}
	return &session.Event{
		Actor:     entry.AgentId,
		Kind:      session.EventKindPlanApproved,
		Plan:      payload,
		Timestamp: entry.Timestamp,
	}
}

func subagentResultEvent(entry *Entry, block *ContentBlock, text string, isDenied bool) *session.Event {
	if isDenied {
		return permissionDeniedEvent(entry, toolNameAgent)
	}

	content := resolvePersistedOutput(text, entry.SessionId, block.ToolUseId)
	if len(content) > maxSubagentResultBytes {
		content = content[:maxSubagentResultBytes] + "\n[peek: subagent result truncated at 32 KB]\n"
	}

	payload := &session.SubagentPayload{
		Content:   content,
		IsError:   block.IsError,
		ToolUseId: block.ToolUseId,
	}
	return &session.Event{
		Actor:     entry.AgentId,
		Kind:      session.EventKindSubagentResult,
		Subagent:  payload,
		Timestamp: entry.Timestamp,
	}
}

func userAnswerEvent(entry *Entry, block *ContentBlock, pending *pendingToolUse, text string, isDenied bool) *session.Event {
	if isDenied {
		return permissionDeniedEvent(entry, toolNameAskUserQuestion)
	}

	var input askUserQuestionInput
	if err := json.Unmarshal(pending.input, &input); err != nil {
		slog.Debug("userAnswerEvent: unmarshal", "err", err)
	}

	questions := make([]string, 0, len(input.Questions))
	for _, question := range input.Questions {
		questions = append(questions, question.Question)
	}

	payload := &session.UserAnswerPayload{Answers: text, Questions: questions}
	return &session.Event{
		Actor:      entry.AgentId,
		Kind:       session.EventKindUserAnswer,
		Timestamp:  entry.Timestamp,
		UserAnswer: payload,
	}
}

func permissionDeniedEvent(entry *Entry, tool string) *session.Event {
	payload := &session.PermissionPayload{Tool: tool}
	return &session.Event{
		Actor:      entry.AgentId,
		Kind:       session.EventKindPermissionDenied,
		Permission: payload,
		Timestamp:  entry.Timestamp,
	}
}

func skillEvent(entry *Entry, block *ContentBlock) *session.Event {
	var input skillInput
	if err := json.Unmarshal(block.Input, &input); err != nil {
		slog.Debug("skillEvent: unmarshal", "err", err)
	}

	payload := &session.SkillPayload{
		Args:   input.Args,
		Skill:  input.Skill,
		Source: session.SkillSourceTool,
	}
	return &session.Event{
		Actor:     entry.AgentId,
		Kind:      session.EventKindSkillInvoked,
		Skill:     payload,
		Timestamp: entry.Timestamp,
	}
}

func slashCommandEvent(entry *Entry, text string) *session.Event {
	name := textBetween(text, commandNameOpenTag, commandNameCloseTag)
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	if name == "" {
		return nil
	}

	args := strings.TrimSpace(textBetween(text, commandArgsOpenTag, commandArgsCloseTag))
	payload := &session.SkillPayload{
		Args:   args,
		Skill:  name,
		Source: session.SkillSourceSlash,
	}
	return &session.Event{
		Actor:     entry.AgentId,
		Kind:      session.EventKindSkillInvoked,
		Skill:     payload,
		Timestamp: entry.Timestamp,
	}
}

func planModeEvents(entry *Entry, attachmentType string) []*session.Event {
	var kind session.EventKind
	switch attachmentType {
	case AttachmentTypePlanMode:
		kind = session.EventKindPlanModeEnter
	case AttachmentTypePlanModeExit:
		kind = session.EventKindPlanModeExit
	case AttachmentTypePlanModeReentry:
		kind = session.EventKindPlanModeReenter
	}

	if kind == "" {
		return nil
	}

	event := &session.Event{
		Actor:     entry.AgentId,
		Kind:      kind,
		Timestamp: entry.Timestamp,
	}
	return []*session.Event{event}
}

// resolvePersistedOutput follows a <persisted-output> pointer, but only into
// the session's own tool-results directory — the pointer text is
// attacker-influenceable content from a tool result.
func resolvePersistedOutput(text string, sessionId session.Id, toolUseId string) string {
	if !strings.HasPrefix(text, persistedOutputMarker) {
		return text
	}

	path := persistedOutputPath(text)
	if !isSessionToolResultPath(path, sessionId, toolUseId) {
		return text
	}

	file, err := os.Open(path)
	if err != nil {
		slog.Debug("resolvePersistedOutput: open", "err", err)
		return text
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxPersistedReadBytes))
	if err != nil {
		slog.Debug("resolvePersistedOutput: read", "err", err)
		return text
	}
	return string(data)
}

func persistedOutputPath(text string) string {
	_, after, found := strings.Cut(text, "Full output saved to: ")
	if !found {
		return ""
	}

	path, _, _ := strings.Cut(after, "\n")
	return strings.TrimSpace(path)
}

func isSessionToolResultPath(path string, sessionId session.Id, toolUseId string) bool {
	if path == "" || sessionId == "" {
		return false
	}
	if filepath.Base(path) != toolUseId+".txt" {
		return false
	}

	parent := filepath.Dir(path)
	if filepath.Base(parent) != toolResultsDir {
		return false
	}

	return filepath.Base(filepath.Dir(parent)) == string(sessionId)
}

func textBetween(text, openTag, closeTag string) string {
	_, after, found := strings.Cut(text, openTag)
	if !found {
		return ""
	}

	inner, _, found := strings.Cut(after, closeTag)
	if !found {
		return ""
	}
	return inner
}

func isPlanAttachment(t string) bool {
	switch t {
	case AttachmentTypePlanMode, AttachmentTypePlanFileReference,
		AttachmentTypePlanModeExit, AttachmentTypePlanModeReentry:
		return true
	}
	return false
}

// originFromEntry returns nil when the entry carries no version, so an empty
// Origin never replaces a populated one via Meta.Update.
func originFromEntry(entry *Entry) *session.Origin {
	if entry.Version == "" {
		return nil
	}
	return &session.Origin{CliVersion: entry.Version}
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
