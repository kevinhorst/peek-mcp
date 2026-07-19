package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMeta_Update_Origin(t *testing.T) {
	existing := &Origin{CliVersion: "1.0.0"}
	meta := &Meta{SessionId: "s1", Origin: existing}

	// nil other.Origin keeps the existing one
	meta.Update(&Meta{SessionId: "s1"})
	assert.Same(t, existing, meta.Origin)

	// non-nil other.Origin replaces wholesale
	replacement := &Origin{CliVersion: "2.0.0", Originator: "Codex Desktop"}
	meta.Update(&Meta{SessionId: "s1", Origin: replacement})
	assert.Same(t, replacement, meta.Origin)
}
