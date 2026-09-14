package session

import (
	"errors"
	"maps"
	"math"
	"slices"
	"time"
)

type (
	Id          string
	Agent       string
	TitleSource string
)

const (
	AgentClaude Agent = "claude"
	AgentCodex  Agent = "codex"
)

const (
	TitleSourceCustom  TitleSource = "custom"
	TitleSourceDerived TitleSource = "derived"
	TitleSourceIndex   TitleSource = "index"
)

const EventBufferCapacity = 500

const idleThreshold = 5 * time.Minute

const maxTouchedFiles = 2000

const maxSubagentStats = 1000

const subagentTurnDepth = 200

const AllTurns = math.MaxInt

const maxSkillStats = 200

const StopReasonToolUse = "tool_use"

type DiffSource string

const (
	DiffSourceLive     DiffSource = "live"
	DiffSourceSnapshot DiffSource = "snapshot"
)

type Session struct {
	activeSkills     map[string]*SkillStat
	currentPromptId  string
	planExitSeen     bool
	usageByRequestId map[string]*requestUsage

	Agent           Agent                       `json:"agent"`
	Counters        Counters                    `json:"-"`
	DiffBase        string                      `json:"-"`
	DiffCapturedAt  time.Time                   `json:"-"`
	DiffOutput      string                      `json:"-"`
	DiffSource      DiffSource                  `json:"-"`
	DiffTarget      string                      `json:"diff_target,omitempty"`
	Events          *EventBuffer                `json:"-"`
	FilePath        string                      `json:"-"`
	HasDiffSnapshot bool                        `json:"-"`
	Idle            time.Duration               `json:"-"`
	LastActive      time.Time                   `json:"last_active"`
	Meta            Meta                        `json:"meta"`
	PlanContent     string                      `json:"-"`
	PlanFilePath    string                      `json:"-"`
	PlanRevisions   []*PlanRevision             `json:"-"`
	Skills          []*SkillStat                `json:"-"`
	StartedAt       time.Time                   `json:"-"`
	Subagents       map[string]*SubagentStat    `json:"-"`
	Title           string                      `json:"title,omitempty"`
	TitleSource     TitleSource                 `json:"title_source,omitempty"`
	TotalUsage      Usage                       `json:"total_usage"`
	TouchedFiles    map[string]*FileTouchCounts `json:"-"`
	TurnActive      *Turn                       `json:"-"`
	TurnsFinished   *TurnBuffer
	UncommittedDiff string `json:"-"`
}

func (s *Session) isAlterationPhase() bool {
	if s.Agent == AgentCodex {
		return len(s.PlanRevisions) >= 1
	}
	return s.planExitSeen
}

func (s *Session) AddEvent(event *Event) {
	s.Events.Push(event)

	switch event.Kind {
	case EventKindModelChanged:
		s.Counters.ModelChanges++
	case EventKindPermissionDenied:
		s.Counters.PermissionDenials++
	case EventKindPermissionGranted:
		s.Counters.PermissionGrants++
	case EventKindPermissionModeChanged:
		s.Counters.PermissionModeChanges++
	case EventKindPlanModeExit:
		s.planExitSeen = true
	case EventKindPlanRejected:
		s.Counters.PlanRejections++
	case EventKindSkillInvoked:
		s.Counters.SkillsInvoked++
		if event.Skill != nil {
			s.openSkillWindow(event)
		}
	case EventKindSubagentSpawned:
		s.Counters.SubagentsSpawned++
	case EventKindPlanApproved, EventKindPlanModeEnter, EventKindPlanModeReenter,
		EventKindPlanRevised, EventKindSubagentResult, EventKindUserAnswer:
	}
}

type FileTouchCounts struct {
	Reads  int `json:"reads"`
	Writes int `json:"writes"`
}

func (s *Session) AddFileTouch(touch *FileTouch) {
	s.TouchedFiles = addFileTouch(s.TouchedFiles, touch)
}

func addFileTouch(files map[string]*FileTouchCounts, touch *FileTouch) map[string]*FileTouchCounts {
	if files == nil {
		files = make(map[string]*FileTouchCounts)
	}

	counts, ok := files[touch.Path]
	if !ok {
		if len(files) >= maxTouchedFiles {
			return files
		}
		counts = &FileTouchCounts{}
		files[touch.Path] = counts
	}

	if touch.Write {
		counts.Writes++
		return files
	}
	counts.Reads++
	return files
}

type SkillStat struct {
	AgentId   string    `json:"agent_id,omitempty"`
	Skill     string    `json:"skill"`
	Args      string    `json:"args,omitempty"`
	Model     string    `json:"model,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Usage     Usage     `json:"usage"`
}

func (s *Session) HandlePromptBoundary(promptId string, timestamp time.Time) {
	isSameSubmission := promptId != "" && promptId == s.currentPromptId
	s.currentPromptId = promptId
	if isSameSubmission {
		return
	}
	s.CloseSkillWindow(timestamp)
}

func (s *Session) CloseSkillWindow(timestamp time.Time) {
	s.closeSkillWindow("", timestamp)
}

func (s *Session) closeSkillWindow(actor string, timestamp time.Time) {
	stat, ok := s.activeSkills[actor]
	if !ok {
		return
	}
	if stat.EndedAt.IsZero() && !timestamp.IsZero() {
		stat.EndedAt = timestamp
	}
	delete(s.activeSkills, actor)
}

func (s *Session) openSkillWindow(event *Event) {
	s.closeSkillWindow(event.Actor, event.Timestamp)
	if len(s.Skills) >= maxSkillStats {
		return
	}
	if s.activeSkills == nil {
		s.activeSkills = make(map[string]*SkillStat)
	}

	stat := &SkillStat{
		AgentId:   event.Actor,
		Skill:     event.Skill.Skill,
		Args:      event.Skill.Args,
		StartedAt: event.Timestamp,
	}
	s.Skills = append(s.Skills, stat)
	s.activeSkills[event.Actor] = stat
}

type SubagentStat struct {
	AgentType    string                      `json:"agent_type,omitempty"`
	Description  string                      `json:"description,omitempty"`
	FirstActive  time.Time                   `json:"first_active"`
	LastActive   time.Time                   `json:"last_active"`
	Model        string                      `json:"model,omitempty"`
	TouchedFiles map[string]*FileTouchCounts `json:"-"`
	TurnActive   *Turn                       `json:"-"`
	Turns        *TurnBuffer                 `json:"-"`
	Usage        Usage                       `json:"usage"`
}

func (s *Session) AddSubagentTurn(turn *Turn) {
	if s.Subagents == nil {
		s.Subagents = make(map[string]*SubagentStat)
	}

	stat, ok := s.Subagents[turn.SubagentId]
	if !ok {
		if len(s.Subagents) >= maxSubagentStats {
			return
		}
		stat = &SubagentStat{Turns: NewTurnBuffer(subagentTurnDepth)}
		s.Subagents[turn.SubagentId] = stat
	}

	if turn.Role != "" {
		stat.TurnActive = appendTurn(stat.TurnActive, stat.Turns, turn)
	}

	if turn.Meta != nil && turn.Meta.Model != "" {
		stat.Model = turn.Meta.Model
	}

	if !turn.Timestamp.IsZero() {
		if stat.FirstActive.IsZero() {
			stat.FirstActive = turn.Timestamp
		}
		stat.LastActive = turn.Timestamp
	}

	for _, event := range turn.Events {
		if event.Kind == EventKindSubagentSpawned && event.Subagent != nil {
			stat.AgentType = event.Subagent.AgentType
			stat.Description = event.Subagent.Description
		}
	}

	for _, touch := range turn.FileTouches {
		stat.TouchedFiles = addFileTouch(stat.TouchedFiles, touch)
	}

	if turn.Usage == nil || turn.RequestId == "" {
		return
	}
	targets := []*Usage{&stat.Usage}
	if skill, ok := s.activeSkills[turn.SubagentId]; ok {
		targets = append(targets, &skill.Usage)
		skill.EndedAt = turn.Timestamp
		if skill.Model == "" && turn.Meta != nil {
			skill.Model = turn.Meta.Model
		}
	}
	s.applyRequestUsage(turn.RequestId, turn.Usage, targets)
}

type requestUsage struct {
	counted Usage
	targets []*Usage
}

func (s *Session) applyRequestUsage(requestId string, usage *Usage, targets []*Usage) {
	if s.usageByRequestId == nil {
		s.usageByRequestId = make(map[string]*requestUsage)
	}
	if previous, counted := s.usageByRequestId[requestId]; counted {
		for _, target := range previous.targets {
			target.Sub(&previous.counted)
		}
	}
	for _, target := range targets {
		target.Add(usage)
	}
	s.usageByRequestId[requestId] = &requestUsage{counted: *usage, targets: targets}
}

func (s *Session) AddTurn(nextTurn *Turn) {
	// always update meta info
	s.Meta.Update(nextTurn.Meta)

	if !nextTurn.Timestamp.IsZero() {
		if s.StartedAt.IsZero() {
			s.StartedAt = nextTurn.Timestamp
		}
		if gap := nextTurn.Timestamp.Sub(s.LastActive); !s.LastActive.IsZero() && gap >= idleThreshold {
			s.Idle += gap
		}
		s.LastActive = nextTurn.Timestamp
	}

	if nextTurn.Usage != nil && nextTurn.RequestId != "" {
		targets := []*Usage{&s.TotalUsage}
		if skill, ok := s.activeSkills[""]; ok {
			targets = append(targets, &skill.Usage)
			skill.EndedAt = nextTurn.Timestamp
			if skill.Model == "" {
				skill.Model = nextTurn.Meta.Model
			}
		}
		s.applyRequestUsage(nextTurn.RequestId, nextTurn.Usage, targets)
	}

	if nextTurn.StopReason != "" && nextTurn.StopReason != StopReasonToolUse {
		s.CloseSkillWindow(nextTurn.Timestamp)
	}

	s.TurnActive = appendTurn(s.TurnActive, s.TurnsFinished, nextTurn)
}

// appendTurn merges streaming chunks of the same request into the active turn
// and pushes the finished turn to the buffer when a new request begins.
func appendTurn(active *Turn, buffer *TurnBuffer, next *Turn) *Turn {
	if active == nil {
		return next
	}

	if next.RequestId != "" && active.RequestId == next.RequestId {
		merged := *next
		merged.Text = active.Text + next.Text
		merged.Thinking = active.Thinking + next.Thinking
		return &merged
	}

	if active.Text != "" || active.Thinking != "" {
		buffer.Push(active)
	}

	return next
}

func (s *Session) CurrentUsage() *Usage {
	total := s.TotalUsage
	return &total
}

func (s *Session) HasNewTitle(title string, source TitleSource) bool {
	if title == "" {
		return false
	}

	if titleSourceRank(source) < titleSourceRank(s.TitleSource) {
		return false
	}

	return s.Title != title
}

// TotalTurns is the total turn count of the session, counted at parse time —
// unlike the ring-capped TurnsFinished buffer it never loses old turns.
func (s *Session) TotalTurns() int {
	total := s.TurnsFinished.Pushed()
	if s.TurnActive != nil {
		total++
	}
	return total
}

func (s *Session) Turns(number int) []*Turn {
	return lastTurns(s.TurnActive, s.TurnsFinished, number)
}

func (s *Session) SubagentTurns(agentId string, number int) ([]*Turn, bool) {
	stat, ok := s.Subagents[agentId]
	if !ok {
		return nil, false
	}

	return lastTurns(stat.TurnActive, stat.Turns, number), true
}

func lastTurns(active *Turn, finished *TurnBuffer, number int) []*Turn {
	if active == nil {
		return finished.Last(number)
	}

	buffer := &TurnBuffer{
		capacity: finished.capacity,
		items:    append(slices.Clone(finished.items), active),
	}

	return buffer.Last(number)
}

func (s *Session) SubagentIds() []string {
	ids := slices.Collect(maps.Keys(s.Subagents))
	slices.Sort(ids)
	return ids
}

func (s *Session) Validate() error {
	if s == nil {
		return errors.New("Session.Validate: called on nil")
	}

	if s.Meta.SessionId == "" {
		return errors.New("Session.Validate: id must not be empty")
	}

	if s.Agent != AgentClaude && s.Agent != AgentCodex {
		return errors.New("Session.Validate: agent must be \"claude\" or \"codex\"")
	}

	if s.LastActive.IsZero() {
		return errors.New("Session.Validate: last_active must not be zero")
	}

	if s.TurnsFinished == nil {
		return errors.New("Session.Validate: turns must not be nil")
	}

	if s.Events == nil {
		return errors.New("Session.Validate: events must not be nil")
	}

	return nil
}

func titleSourceRank(source TitleSource) int {
	switch source {
	case TitleSourceCustom:
		return 2
	case TitleSourceIndex:
		return 1
	case TitleSourceDerived:
		return 0
	}
	return 0
}
