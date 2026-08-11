package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPageBuilder_Build_SinglePage(t *testing.T) {
	first, next := NewPageBuilder(100).build("turns", "plan", "diff", "uncommitted")

	assert.Empty(t, next)
	assert.Equal(t, "turns", first.Turns)
	assert.Equal(t, "plan", first.Plan)
	assert.Equal(t, "diff", first.Diff)
	assert.Equal(t, "uncommitted", first.UncommittedDiff)
}

func TestPageBuilder_Build_DrainOrder(t *testing.T) {
	first, next := NewPageBuilder(4).build("tttt", "pppp", "dddd", "uuuu")

	pages := append([]*sessionGetResult{first}, next...)
	assert.Len(t, pages, 4)
	assert.Equal(t, "tttt", pages[0].Turns)
	assert.Equal(t, "pppp", pages[1].Plan)
	assert.Equal(t, "dddd", pages[2].Diff)
	assert.Equal(t, "uuuu", pages[3].UncommittedDiff)
}

func TestPageBuilder_Build_MixedPageBoundary(t *testing.T) {
	first, next := NewPageBuilder(6).build("tttt", "pp", "dddd", "uu")

	pages := append([]*sessionGetResult{first}, next...)
	assert.Len(t, pages, 2)
	assert.Equal(t, "tttt", pages[0].Turns)
	assert.Equal(t, "pp", pages[0].Plan)
	assert.Equal(t, "dddd", pages[1].Diff)
	assert.Equal(t, "uu", pages[1].UncommittedDiff)
}
