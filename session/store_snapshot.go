package session

import (
	"encoding/gob"
	"io"

	"github.com/pkg/errors"
)

type usageTargetKind int

const (
	usageTargetSession usageTargetKind = iota + 1
	usageTargetSkill
	usageTargetSubagent
)

// FileState is one transcript file's read position and its parser's cross-line state.
type FileState struct {
	Offset int64
	Parser []byte
}

type requestUsageSnapshot struct {
	Counted Usage
	Targets []*usageTarget
}

func (s *requestUsageSnapshot) restore(session *Session) *requestUsage {
	usage := &requestUsage{counted: s.Counted}
	for _, target := range s.Targets {
		resolved := target.resolve(session)
		if resolved == nil {
			continue
		}

		usage.targets = append(usage.targets, resolved)
	}
	return usage
}

type sessionSnapshot struct {
	ActiveSkills    map[string]int
	CurrentPromptId string
	PlanExitSeen    bool
	RequestUsage    map[string]*requestUsageSnapshot
	Session         *Session
}

func newSessionSnapshot(session *Session) *sessionSnapshot {
	return &sessionSnapshot{
		ActiveSkills:    activeSkillIndexes(session),
		CurrentPromptId: session.currentPromptId,
		PlanExitSeen:    session.planExitSeen,
		RequestUsage:    requestUsageSnapshots(session),
		Session:         session,
	}
}

func (s *sessionSnapshot) restore() *Session {
	session := s.Session
	session.currentPromptId = s.CurrentPromptId
	session.planExitSeen = s.PlanExitSeen

	session.activeSkills = make(map[string]*SkillStat, len(s.ActiveSkills))
	for actor, index := range s.ActiveSkills {
		session.activeSkills[actor] = session.Skills[index]
	}

	session.usageByRequestId = make(map[string]*requestUsage, len(s.RequestUsage))
	for requestId, usage := range s.RequestUsage {
		session.usageByRequestId[requestId] = usage.restore(session)
	}
	return session
}

// StoreSnapshot is a Store's serial form together with the watcher file states it was built from.
type StoreSnapshot struct {
	Files          map[string]FileState
	PlainTitleById map[Id]string
	Sessions       []*sessionSnapshot
}

type usageTarget struct {
	Kind       usageTargetKind
	SkillIndex int
	SubagentId string
}

func (t *usageTarget) resolve(session *Session) *Usage {
	switch t.Kind {
	case usageTargetSession:
		return &session.TotalUsage
	case usageTargetSkill:
		return &session.Skills[t.SkillIndex].Usage
	case usageTargetSubagent:
		return &session.Subagents[t.SubagentId].Usage
	}
	return nil
}

func activeSkillIndexes(session *Session) map[string]int {
	indexBySkill := make(map[*SkillStat]int, len(session.Skills))
	for index, skill := range session.Skills {
		indexBySkill[skill] = index
	}

	indexes := make(map[string]int, len(session.activeSkills))
	for actor, skill := range session.activeSkills {
		indexes[actor] = indexBySkill[skill]
	}
	return indexes
}

// requestUsageSnapshots skips a target outside the session's usage totals, as gob refuses nil slice elements.
func requestUsageSnapshots(session *Session) map[string]*requestUsageSnapshot {
	targetByUsage := usageTargets(session)
	snapshots := make(map[string]*requestUsageSnapshot, len(session.usageByRequestId))
	for requestId, usage := range session.usageByRequestId {
		snapshot := &requestUsageSnapshot{Counted: usage.counted}
		for _, target := range usage.targets {
			key, ok := targetByUsage[target]
			if !ok {
				continue
			}

			snapshot.Targets = append(snapshot.Targets, key)
		}
		snapshots[requestId] = snapshot
	}
	return snapshots
}

func usageTargets(session *Session) map[*Usage]*usageTarget {
	targets := make(map[*Usage]*usageTarget)
	targets[&session.TotalUsage] = &usageTarget{Kind: usageTargetSession}
	for index, skill := range session.Skills {
		targets[&skill.Usage] = &usageTarget{Kind: usageTargetSkill, SkillIndex: index}
	}
	for id, stat := range session.Subagents {
		targets[&stat.Usage] = &usageTarget{Kind: usageTargetSubagent, SubagentId: id}
	}
	return targets
}

func ReadStoreSnapshot(reader io.Reader) (*StoreSnapshot, error) {
	snapshot := &StoreSnapshot{}
	if err := gob.NewDecoder(reader).Decode(snapshot); err != nil {
		return nil, errors.Wrap(err, "ReadStoreSnapshot: Failed to decode")
	}
	return snapshot, nil
}
