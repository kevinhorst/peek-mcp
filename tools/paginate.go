package tools

import (
	"unicode/utf8"

	"github.com/kevinhorst/peek-mcp/session"
)

// TODO: implement pagination

func buildPages(turns []*session.Turn, plan string, diff string) []*sessionFullResult {
	panic("not implemented")
}

func nextPage(requestId string) (page *sessionFullResult, hasMore bool, found bool) {
	panic("not implemented")
}

func storePagination(pages []*sessionFullResult) *sessionFullResultPaginated {
	panic("not implemented")
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
