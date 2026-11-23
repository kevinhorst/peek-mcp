package tools

import (
	"log/slog"
	"slices"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/watcher"
)

// TurnSource is what a turns request needs to reach past the buffer, copied out under the store lock.
type TurnSource struct {
	agent      session.Agent
	depth      int
	id         session.Id
	paths      []string
	subagentId string
	total      int
	withTools  bool
}

func NewTurnSource(sess *session.Session, subagentId string, withTools bool) *TurnSource {
	source := &TurnSource{
		agent:      sess.Agent,
		id:         sess.Meta.SessionId,
		paths:      slices.Clone(sess.FilePaths),
		subagentId: subagentId,
		total:      scopeTurnsTotal(sess, subagentId, withTools),
		withTools:  withTools,
	}

	if subagentId == "" {
		source.depth = sess.TotalTurnsWithToolCalls() + 1
		return source
	}

	source.depth = sess.SubagentTotalTurnsWithToolCalls(subagentId) + 1
	stat, ok := sess.Subagents[subagentId]
	isOwnTranscript := ok && stat.FilePath != "" && !slices.Contains(source.paths, stat.FilePath)
	if isOwnTranscript {
		source.paths = append(source.paths, stat.FilePath)
	}
	return source
}

// HasOlder reports whether the scope holds more turns than the buffer returned.
func (s *TurnSource) HasOlder(buffered int) bool {
	return s.total > buffered
}

// Turns returns the buffered turns, or number turns re-read from the transcripts when the buffer holds fewer than asked and more exist.
func (s *TurnSource) Turns(loader *watcher.TurnLoader, buffered []*session.Turn, number int) []*session.Turn {
	isShort := len(buffered) < number && s.HasOlder(len(buffered))
	isLoadable := loader != nil && len(s.paths) > 0
	if !isShort || !isLoadable {
		return buffered
	}

	loaded, err := loader.Load(s.agent, s.depth, s.id, s.paths...)
	if err != nil {
		slog.Warn("TurnSource.Turns: Failed to load transcript", "session", s.id, "err", err)
		return buffered
	}

	turns, ok := scopeTurns(loaded, number, s.subagentId, s.withTools)
	if !ok {
		return buffered
	}
	return turns
}
