package tools

import (
	"encoding/json"
	"log/slog"
	"math"
	"sync"
	"unicode/utf8"
)

type PageStore[T any] struct {
	mu               sync.Mutex
	PagesByRequestId map[string]<-chan T
}

func (s *PageStore[T]) add(requestId string, results []T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	queue := make(chan T, len(results))
	for _, result := range results {
		queue <- result
	}

	close(queue)
	s.PagesByRequestId[requestId] = queue
}

func (s *PageStore[T]) hasNext(requestId string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, ok := s.PagesByRequestId[requestId]
	if !ok {
		return false
	}

	return len(result) > 0
}

func (s *PageStore[T]) next(requestId string) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, ok := s.PagesByRequestId[requestId]
	if !ok {
		var zero T
		return zero, false
	}

	return <-result, true
}

func (s *PageStore[T]) remove(requestId string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.PagesByRequestId, requestId)
}

type PageBuilder struct {
	Size int
}

func NewPageBuilder(size int) *PageBuilder {
	return &PageBuilder{Size: size}
}

func (b *PageBuilder) build(diff, events, memory, plan, turns string) (first *sessionFullResult, next []*sessionFullResult) {
	// Check if everything fits in a single page
	contentSize := len(turns) + len(events) + len(plan) + len(diff) + len(memory)
	if b.Size <= 0 || contentSize <= b.Size {
		slog.Info("PageBuilder.build: fits in a single page", "size", contentSize, "page_size", b.Size)
		first = &sessionFullResult{
			Diff:   diff,
			Events: events,
			Memory: memory,
			Plan:   plan,
			Turns:  turns,
		}
		return first, nil
	}

	// Check how many pages we need to build, round up
	pageCount := math.Ceil(float64(contentSize) / float64(b.Size))
	pages := make([]*sessionFullResult, int(pageCount))
	slog.Info("PageBuilder.build: building", "pageCount", pageCount, "size", b.Size)

	for pageIndex := 0; pageIndex < int(pageCount); pageIndex++ {
		page := &sessionFullResult{}
		pages[pageIndex] = page
		size := b.Size

		// drain turns, events, plan, diff, and memory into pages by priority
		turnChunk := utf8SafeSlice(turns, size)
		page.Turns = turnChunk
		turns = turns[len(turnChunk):]
		if len(turnChunk) == size {
			continue
		}
		size = size - len(turnChunk)

		eventChunk := utf8SafeSlice(events, size)
		page.Events = eventChunk
		events = events[len(eventChunk):]
		if len(eventChunk) == size {
			continue
		}
		size = size - len(eventChunk)

		planChunk := utf8SafeSlice(plan, size)
		page.Plan = planChunk
		plan = plan[len(planChunk):]
		if len(planChunk) == size {
			continue
		}
		size = size - len(planChunk)

		diffChunk := utf8SafeSlice(diff, size)
		page.Diff = diffChunk
		diff = diff[len(diffChunk):]
		if len(diffChunk) == size {
			continue
		}
		size = size - len(diffChunk)

		memoryChunk := utf8SafeSlice(memory, size)
		page.Memory = memoryChunk
		memory = memory[len(memoryChunk):]
	}

	return pages[0], pages[1:]
}

func (b *PageBuilder) buildEvents(events, revisions string) (first *sessionEventsResult, next []*sessionEventsResult) {
	contentSize := len(events) + len(revisions)
	if b.Size <= 0 || contentSize <= b.Size {
		first = &sessionEventsResult{
			Events:    rawJsonSegment(events),
			Revisions: rawJsonSegment(revisions),
		}
		return first, nil
	}

	pageCount := math.Ceil(float64(contentSize) / float64(b.Size))
	pages := make([]*sessionEventsResult, int(pageCount))
	slog.Info("PageBuilder.buildEvents: building", "pageCount", pageCount, "size", b.Size)

	for pageIndex := 0; pageIndex < int(pageCount); pageIndex++ {
		page := &sessionEventsResult{}
		pages[pageIndex] = page
		size := b.Size

		eventChunk := utf8SafeSlice(events, size)
		page.Events = rawJsonSegment(eventChunk)
		events = events[len(eventChunk):]
		if len(eventChunk) == size {
			continue
		}
		size = size - len(eventChunk)

		revisionChunk := utf8SafeSlice(revisions, size)
		page.Revisions = rawJsonSegment(revisionChunk)
		revisions = revisions[len(revisionChunk):]
	}

	return pages[0], pages[1:]
}

func rawJsonSegment(segment string) json.RawMessage {
	if segment == "" {
		return nil
	}
	if json.Valid([]byte(segment)) {
		return json.RawMessage(segment)
	}

	quoted, err := json.Marshal(segment)
	if err != nil {
		return nil
	}
	return quoted
}

func utf8SafeSlice(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}

	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
