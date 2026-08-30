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
	toolNameEdit            = "Edit"
	toolNameExitPlanMode    = "ExitPlanMode"
	toolNameMultiEdit       = "MultiEdit"
	toolNameNotebookEdit    = "NotebookEdit"
	toolNameRead            = "Read"
	toolNameSkill           = "Skill"
	toolNameWrite           = "Write"

	syntheticModel = "<synthetic>"

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

type askUserQuestion struct {
	Question string `json:"question"`
}

type askUserQuestionInput struct {
	Questions []askUserQuestion `json:"questions"`
}

type Parser struct {
	pendingTools   map[string]*pendingToolUse
	permissionMode string
}

func NewParser() *Parser {
	return &Parser{pendingTools: make(map[string]*pendingToolUse)}
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
	if len(entry.Message) == 0 {
		return nil
	}

	var message Message
	if err := json.Unmarshal(entry.Message, &message); err != nil {
		slog.Debug("handleUser: unmarshal", "err", err)
		return nil
	}
	if err := message.Validate(); err != nil {
		slog.Debug("handleUser: validate", "err", err)
		return nil
	}

	events, touches := p.eventsFromUserContent(entry, &message)
	if event := p.permissionModeEvent(entry); event != nil {
		events = append(events, event)
	}

	text := extractTextBlocks(message.Content)
	if event := slashCommandEvent(entry, text); event != nil {
		events = append(events, event)
	}

	isPrompt := entry.PromptId != "" && strings.TrimSpace(text) != ""
	if !isPrompt {
		return eventTurn(entry, events, touches)
	}

	turn := &session.Turn{
		Events:      events,
		FileTouches: touches,
		PromptId:    entry.PromptId,
		Role:        session.RoleUser,
		Text:        text,
		Timestamp:   entry.Timestamp,
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
	thinking := extractThinkingBlocks(message.Content)
	events := p.eventsFromAssistantContent(entry, &message)

	model := message.Model
	if model == syntheticModel {
		model = ""
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
	turn := &session.Turn{
		Events:     events,
		Role:       session.RoleAssistant,
		Text:       text,
		Thinking:   thinking,
		Timestamp:  entry.Timestamp,
		RequestId:  entry.RequestId,
		StopReason: message.StopReason,
		Usage:      usage,
		Meta: &session.Meta{
			SessionId: entry.SessionId,
			CWD:       entry.CurrentWorkingDir,
			GitBranch: entry.GitBranch,
			Model:     model,
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

	events := planModeEvents(attachment.Type, entry)

	if attachment.PlanFilePath == "" {
		return eventTurn(entry, events, nil)
	}

	return &session.Turn{
		Events:       events,
		PlanFilePath: attachment.PlanFilePath,
		PlanContent:  attachment.PlanContent,
		Timestamp:    entry.Timestamp,
		Meta: &session.Meta{
			SessionId: entry.SessionId,
			CWD:       entry.CurrentWorkingDir,
		},
	}
}

func (p *Parser) handleSidechain(entry *Entry) *session.Turn {
	if entry.AgentId == "" {
		return nil
	}

	var message Message
	if err := json.Unmarshal(entry.Message, &message); err != nil {
		return nil
	}

	var events []*session.Event
	var touches []*session.FileTouch
	var usage *session.Usage
	var role session.Role
	var text, thinking, model string
	switch entry.Type {
	case EntryTypeUser:
		events, touches = p.eventsFromUserContent(entry, &message)
		role = session.RoleUser
		text = extractTextBlocks(message.Content)
	case EntryTypeAssistant:
		events = p.eventsFromAssistantContent(entry, &message)
		role = session.RoleAssistant
		text = extractTextBlocks(message.Content)
		thinking = extractThinkingBlocks(message.Content)
		if message.Model != syntheticModel {
			model = message.Model
		}
		if message.Usage != nil {
			usage = &session.Usage{
				InputTokens:              message.Usage.InputTokens,
				OutputTokens:             message.Usage.OutputTokens,
				CacheCreationInputTokens: message.Usage.CacheCreationInputTokens,
				CacheReadInputTokens:     message.Usage.CacheReadInputTokens,
			}
		}
	}

	return &session.Turn{
		Events:      events,
		FileTouches: touches,
		RequestId:   entry.RequestId,
		Role:        role,
		Text:        text,
		Thinking:    thinking,
		SubagentId:  entry.AgentId,
		Timestamp:   entry.Timestamp,
		Usage:       usage,
		Meta: &session.Meta{
			SessionId: entry.SessionId,
			CWD:       entry.CurrentWorkingDir,
			Model:     model,
		},
	}
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
			events = append(events, skillEvent(block, entry))
		}
	}

	if len(events) == 0 {
		return nil
	}

	return events
}

func (p *Parser) eventsFromUserContent(entry *Entry, message *Message) ([]*session.Event, []*session.FileTouch) {
	blocks := contentBlocks(message.Content)

	events := make([]*session.Event, 0)
	touches := make([]*session.FileTouch, 0)
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

		if touch := fileTouchFromResult(block, pending); touch != nil {
			touches = append(touches, touch)
		}

		event := toolResultEvent(block, entry, pending)
		if event != nil {
			events = append(events, event)
		}
	}

	return events, touches
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

type pendingToolUse struct {
	input json.RawMessage
	name  string
}

type skillInput struct {
	Args  string `json:"args"`
	Skill string `json:"skill"`
}

type fileToolInput struct {
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
}

func fileTouchFromResult(block *ContentBlock, pending *pendingToolUse) *session.FileTouch {
	if block.IsError {
		return nil
	}

	var isWrite bool
	switch pending.name {
	case toolNameEdit, toolNameMultiEdit, toolNameNotebookEdit, toolNameWrite:
		isWrite = true
	case toolNameRead:
	default:
		return nil
	}

	var input fileToolInput
	if err := json.Unmarshal(pending.input, &input); err != nil {
		return nil
	}

	path := input.FilePath
	if path == "" {
		path = input.NotebookPath
	}
	if path == "" {
		return nil
	}

	return &session.FileTouch{Path: path, Write: isWrite}
}

func contentBlocks(raw json.RawMessage) []ContentBlock {
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	return blocks
}

func eventTurn(entry *Entry, events []*session.Event, touches []*session.FileTouch) *session.Turn {
	if len(events) == 0 && len(touches) == 0 {
		return nil
	}

	turn := &session.Turn{
		Events:      events,
		FileTouches: touches,
		Meta: &session.Meta{
			SessionId: entry.SessionId,
			CWD:       entry.CurrentWorkingDir,
		},
	}
	return turn
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

func extractThinkingBlocks(raw json.RawMessage) string {
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	var builder strings.Builder
	for _, block := range blocks {
		if block.Type != "thinking" || block.Thinking == "" {
			continue
		}

		builder.WriteString(block.Thinking + "\n")
	}
	return builder.String()
}

func isPlanAttachment(t string) bool {
	switch t {
	case AttachmentTypePlanMode, AttachmentTypePlanFileReference,
		AttachmentTypePlanModeExit, AttachmentTypePlanModeReentry:
		return true
	}
	return false
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

// originFromEntry returns nil when the entry carries no version, so an empty
// Origin never replaces a populated one via Meta.Update.
func originFromEntry(entry *Entry) *session.Origin {
	if entry.Version == "" {
		return nil
	}
	return &session.Origin{CliVersion: entry.Version}
}

type deniedToolInput struct {
	Command      string `json:"command"`
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
}

func commandFromInput(pending *pendingToolUse) string {
	var input deniedToolInput
	if err := json.Unmarshal(pending.input, &input); err != nil {
		return ""
	}
	if input.Command != "" {
		return input.Command
	}
	if input.FilePath != "" {
		return input.FilePath
	}
	return input.NotebookPath
}

const permissionModeDefault = "default"

// permissionModeEvent tracks the permissionMode field across main-chain user
// entries; the first observed non-default mode also emits, so a session that
// starts in bypassPermissions is visible.
func (p *Parser) permissionModeEvent(entry *Entry) *session.Event {
	mode := entry.PermissionMode
	if mode == "" || mode == p.permissionMode {
		return nil
	}

	from := p.permissionMode
	p.permissionMode = mode
	if from == "" && mode == permissionModeDefault {
		return nil
	}

	return &session.Event{
		Actor:          entry.AgentId,
		Kind:           session.EventKindPermissionModeChanged,
		PermissionMode: &session.PermissionModePayload{From: from, To: mode},
		Timestamp:      entry.Timestamp,
	}
}

func permissionDeniedEvent(entry *Entry, tool string, command string) *session.Event {
	payload := &session.PermissionPayload{Command: command, Tool: tool}
	return &session.Event{
		Actor:      entry.AgentId,
		Kind:       session.EventKindPermissionDenied,
		Permission: payload,
		Timestamp:  entry.Timestamp,
	}
}

func persistedOutputPath(text string) string {
	_, after, found := strings.Cut(text, "Full output saved to: ")
	if !found {
		return ""
	}

	path, _, _ := strings.Cut(after, "\n")
	return strings.TrimSpace(path)
}

func planModeEvents(attachmentType string, entry *Entry) []*session.Event {
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

func planVerdictEvent(block *ContentBlock, entry *Entry, text string) *session.Event {
	if block.IsError {
		return &session.Event{
			Actor:     entry.AgentId,
			Kind:      session.EventKindPlanRejected,
			Timestamp: entry.Timestamp,
		}
	}

	content := resolvePersistedOutput(entry.SessionId, text, block.ToolUseId)
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

func resolvePersistedOutput(sessionId session.Id, text, toolUseId string) string {
	if !strings.HasPrefix(text, persistedOutputMarker) {
		return text
	}

	path := persistedOutputPath(text)
	if !isSessionToolResultPath(path, sessionId, toolUseId) {
		return text
	}

	file, err := os.Open(path)
	if err != nil {
		slog.Debug("resolvePersistedOutput: Failed to open persisted output", "err", err)
		return text
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxPersistedReadBytes))
	if err != nil {
		slog.Debug("resolvePersistedOutput: Failed to read persisted output", "err", err)
		return text
	}
	return string(data)
}

func skillEvent(block *ContentBlock, entry *Entry) *session.Event {
	var input skillInput
	if err := json.Unmarshal(block.Input, &input); err != nil {
		slog.Debug("skillEvent: Failed to unmarshal skill input", "err", err)
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

const builtinCommandModel = "model"

func slashCommandEvent(entry *Entry, text string) *session.Event {
	name := textBetween(commandNameCloseTag, commandNameOpenTag, text)
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	if name == "" || name == builtinCommandModel {
		return nil
	}

	args := strings.TrimSpace(textBetween(commandArgsCloseTag, commandArgsOpenTag, text))
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

func subagentResultEvent(block *ContentBlock, entry *Entry, isDenied bool, text string) *session.Event {
	if isDenied {
		return permissionDeniedEvent(entry, toolNameAgent, "")
	}

	content := resolvePersistedOutput(entry.SessionId, text, block.ToolUseId)
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

func textBetween(closeTag, openTag, text string) string {
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

func toolResultEvent(block *ContentBlock, entry *Entry, pending *pendingToolUse) *session.Event {
	var text string
	if err := json.Unmarshal(block.Content, &text); err != nil {
		text = extractTextBlocks(block.Content)
	}

	isDenied := block.IsError && strings.HasPrefix(text, denialPrefix)

	switch pending.name {
	case toolNameExitPlanMode:
		return planVerdictEvent(block, entry, text)
	case toolNameAgent:
		return subagentResultEvent(block, entry, isDenied, text)
	case toolNameAskUserQuestion:
		return userAnswerEvent(entry, isDenied, pending, text)
	default:
		if !isDenied {
			return nil
		}
		return permissionDeniedEvent(entry, pending.name, commandFromInput(pending))
	}
}

func userAnswerEvent(entry *Entry, isDenied bool, pending *pendingToolUse, text string) *session.Event {
	if isDenied {
		return permissionDeniedEvent(entry, toolNameAskUserQuestion, "")
	}

	var input askUserQuestionInput
	if err := json.Unmarshal(pending.input, &input); err != nil {
		slog.Debug("userAnswerEvent: Failed to unmarshal question input", "err", err)
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
