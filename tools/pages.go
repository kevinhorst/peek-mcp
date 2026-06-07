package tools

import (
	"sync"
	"unicode/utf8"
)

type PageStore struct {
	mu               sync.Mutex
	PagesByRequestId map[string]<-chan *sessionFullResult
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
	if len(turns+plan+diff) <= b.Size {
		first = &sessionFullResult{
			Turns: turns,
			Plan:  plan,
			Diff:  diff,
		}
		return first, nil
	}
	// Build first page until size is reached
	// ...

	// Build next pages with Size as their max size
	// ...
	return first, next
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
