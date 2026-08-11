package tools

import (
	"log/slog"
	"math"
	"sync"
	"unicode/utf8"
)

type PageStore struct {
	mu               sync.Mutex
	PagesByRequestId map[string]<-chan *sessionGetResult
}

func (s *PageStore) add(requestId string, results []*sessionGetResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	queue := make(chan *sessionGetResult, len(results))
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

func (s *PageStore) next(requestId string) (*sessionGetResult, bool) {
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

func (b *PageBuilder) build(turns, plan, diff, uncommittedDiff string) (first *sessionGetResult, next []*sessionGetResult) {
	contentSize := len(turns) + len(plan) + len(diff) + len(uncommittedDiff)
	if b.Size <= 0 || contentSize <= b.Size {
		slog.Info("PageBuilder.build: fits in a single page", "size", contentSize, "page_size", b.Size)
		first = &sessionGetResult{
			Turns:           turns,
			Plan:            plan,
			Diff:            diff,
			UncommittedDiff: uncommittedDiff,
		}
		return first, nil
	}

	pageCount := math.Ceil(float64(contentSize) / float64(b.Size))
	pages := make([]*sessionGetResult, int(pageCount))
	slog.Info("PageBuilder.build: building", "pageCount", pageCount, "size", b.Size)

	for i := 0; i < int(pageCount); i++ {
		pages[i] = &sessionGetResult{}
		size := b.Size

		turnChunk := UTF8SafeSlice(turns, size)
		pages[i].Turns = turnChunk
		turns = turns[len(turnChunk):]
		if len(turnChunk) == size {
			continue
		}
		size = size - len(turnChunk)

		planChunk := UTF8SafeSlice(plan, size)
		pages[i].Plan = planChunk
		plan = plan[len(planChunk):]
		if len(planChunk) == size {
			continue
		}
		size = size - len(planChunk)

		diffChunk := UTF8SafeSlice(diff, size)
		pages[i].Diff = diffChunk
		diff = diff[len(diffChunk):]
		if len(diffChunk) == size {
			continue
		}
		size = size - len(diffChunk)

		uncommittedChunk := UTF8SafeSlice(uncommittedDiff, size)
		pages[i].UncommittedDiff = uncommittedChunk
		uncommittedDiff = uncommittedDiff[len(uncommittedChunk):]
	}

	return pages[0], pages[1:]
}

func UTF8SafeSlice(s string, maxBytes int) string {
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
