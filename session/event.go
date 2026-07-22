package session

import (
	"time"

	"github.com/pkg/errors"
)

type EventKind string

const (
	EventKindPermissionDenied EventKind = "permission_denied"
	EventKindPlanApproved     EventKind = "plan_approved"
	EventKindPlanModeEnter    EventKind = "plan_mode_enter"
	EventKindPlanModeExit     EventKind = "plan_mode_exit"
	EventKindPlanModeReenter  EventKind = "plan_mode_reenter"
	EventKindPlanRejected     EventKind = "plan_rejected"
	EventKindPlanRevised      EventKind = "plan_revised"
	EventKindSkillInvoked     EventKind = "skill_invoked"
	EventKindSubagentResult   EventKind = "subagent_result"
	EventKindSubagentSpawned  EventKind = "subagent_spawned"
	EventKindUserAnswer       EventKind = "user_answer"
)

const (
	SkillSourceSlash = "slash"
	SkillSourceTool  = "tool"
)

type Counters struct {
	PermissionDenials int `json:"permission_denials"`
	PlanAlterations   int `json:"plan_alterations"`
	PlanRejections    int `json:"plan_rejections"`
	SkillsInvoked     int `json:"skills_invoked"`
	SubagentsSpawned  int `json:"subagents_spawned"`
}

type Event struct {
	Actor      string             `json:"actor,omitempty"`
	Kind       EventKind          `json:"kind"`
	Permission *PermissionPayload `json:"permission,omitempty"`
	Plan       *PlanPayload       `json:"plan,omitempty"`
	Skill      *SkillPayload      `json:"skill,omitempty"`
	Subagent   *SubagentPayload   `json:"subagent,omitempty"`
	Timestamp  time.Time          `json:"timestamp"`
	UserAnswer *UserAnswerPayload `json:"user_answer,omitempty"`
}

func (e *Event) Validate() error {
	if e == nil {
		return errors.New("Event.Validate: called on nil")
	}

	// Kind
	if e.Kind == "" {
		return errors.New("Event.Validate: Missing field Kind")
	}

	return nil
}

type PermissionPayload struct {
	Command       string `json:"command,omitempty"`
	Justification string `json:"justification,omitempty"`
	Tool          string `json:"tool"`
}

type PlanPayload struct {
	Content  string `json:"content,omitempty"`
	Revision int    `json:"revision,omitempty"`
}

type SkillPayload struct {
	Args   string `json:"args,omitempty"`
	Skill  string `json:"skill"`
	Source string `json:"source"`
}

type SubagentPayload struct {
	AgentId     string `json:"agent_id,omitempty"`
	AgentType   string `json:"agent_type,omitempty"`
	Content     string `json:"content,omitempty"`
	Description string `json:"description,omitempty"`
	IsError     bool   `json:"is_error,omitempty"`
	SpawnDepth  int    `json:"spawn_depth,omitempty"`
	ToolUseId   string `json:"tool_use_id,omitempty"`
}

type UserAnswerPayload struct {
	Answers   string   `json:"answers"`
	Questions []string `json:"questions,omitempty"`
}
