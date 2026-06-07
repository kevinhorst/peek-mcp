package cmd

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testPrompter(input string) (*prompter, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return &prompter{
		scanner: bufio.NewScanner(strings.NewReader(input)),
		out:     out,
	}, out
}

func TestConfirm_DefaultYes(t *testing.T) {
	p, _ := testPrompter("\n")
	assert.True(t, p.Confirm("Continue?", true))
}

func TestConfirm_DefaultNo(t *testing.T) {
	p, _ := testPrompter("\n")
	assert.False(t, p.Confirm("Continue?", false))
}

func TestConfirm_ExplicitYes(t *testing.T) {
	p, _ := testPrompter("y\n")
	assert.True(t, p.Confirm("Continue?", false))
}

func TestConfirm_ExplicitNo(t *testing.T) {
	p, _ := testPrompter("n\n")
	assert.False(t, p.Confirm("Continue?", true))
}

func TestChoose_Default(t *testing.T) {
	p, _ := testPrompter("\n")
	got := p.Choose("Pick one:", []string{"A", "B", "C"}, 1)
	assert.Equal(t, 1, got)
}

func TestChoose_ExplicitSelection(t *testing.T) {
	p, _ := testPrompter("3\n")
	got := p.Choose("Pick one:", []string{"A", "B", "C"}, 0)
	assert.Equal(t, 2, got)
}

func TestChoose_OutOfRange(t *testing.T) {
	p, _ := testPrompter("9\n")
	got := p.Choose("Pick one:", []string{"A", "B"}, 0)
	assert.Equal(t, 0, got)
}
