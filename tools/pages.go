package tools

import (
	"log/slog"
	"math"
	"sync"
	"unicode/utf8"
)

type PageStore struct {
	mu               sync.Mutex
	PagesByRequestId map[string]<-chan *sessionFullResult
}

func (s *PageStore) add(requestId string, results []*sessionFullResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	queue := make(chan *sessionFullResult, len(results))
	for _, result := range results {
		queue <- result
	}

	close(queue)
	s.PagesByRequestId[requestId] = queue
}

func (s *PageStore) hasNext(requestId string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, ok := s.PagesByRequestId[requestId]
	if !ok {
		return false
	}

	return len(result) > 0
}

func (s *PageStore) next(requestId string) (*sessionFullResult, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, ok := s.PagesByRequestId[requestId]
	if !ok {
		return nil, false
	}

	return <-result, true
}

func (s *PageStore) remove(requestId string) {
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

func (b *PageBuilder) build(turns, plan, diff string) (first *sessionFullResult, next []*sessionFullResult) {
	// Check if everything fits in a single page
	contentSize := len(turns) + len(plan) + len(diff)
	if contentSize <= b.Size {
		slog.Debug("PageBuilder.build: fits in a single page", "size", contentSize)
		first = &sessionFullResult{
			Turns: turns,
			Plan:  plan,
			Diff:  diff,
		}
		return first, nil
	}

	// Check how many pages we need to build, round up
	pageCount := math.Ceil(float64(contentSize) / float64(b.Size))
	pages := make([]*sessionFullResult, int(pageCount))
	slog.Debug("PageBuilder.build: building", "pageCount", pageCount, "size", b.Size)

	for i := 0; i < int(pageCount); i++ {
		pages[i] = &sessionFullResult{}
		size := b.Size

		// drain turns, plan and diff into pages by priority
		turnChunk := utf8SafeSlice(turns, size)
		pages[i].Turns = turnChunk
		turns = turns[len(turnChunk):]
		if len(turnChunk) == size {
			continue
		}
		size = size - len(turnChunk)

		planChunk := utf8SafeSlice(plan, size)
		pages[i].Plan = planChunk
		plan = plan[len(planChunk):]
		if len(planChunk) == size {
			continue
		}
		size = size - len(planChunk)

		diffChunk := utf8SafeSlice(diff, size)
		pages[i].Diff = diffChunk
		diff = diff[len(diffChunk):]
	}

	return pages[0], pages[1:]
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
