package tools

import (
	"github.com/kevinhorst/peek-mcp/claude"
)

type memoryBlockResult struct {
	Facts       []*claude.MemoryFact `json:"facts,omitempty"`
	Index       string               `json:"index,omitempty"`
	IsTruncated bool                 `json:"truncated,omitempty"`
	Unsupported string               `json:"unsupported,omitempty"`
}
