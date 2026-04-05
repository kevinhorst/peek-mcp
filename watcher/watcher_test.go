package watcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kevinhorst/peek-mcp/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubParser struct {
	lines      []string
	flushCount int
}

func (p *stubParser) ParseLine(line []byte) {
	p.lines = append(p.lines, string(line))
}

func (p *stubParser) Flush() {
	p.flushCount++
}

func provideStubWatcher(path string) (*Watcher, *stubParser) {
	p := &stubParser{}
	w := New(store.New(5), "", "")
	w.files[watchedFilePath(path)] = &watchedFile{
		parser: p,
	}
	return w, p
}

func TestParseCompleteLines(t *testing.T) {
	type testCase struct {
		_id            string
		_buffer        []byte
		_expectedLines []string
		_expectedTail  []byte
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:            "pass-complete-lines",
		_buffer:        []byte("first\nsecond\n"),
		_expectedLines: []string{"first", "second"},
		_expectedTail:  nil,
	}
	tests = append(tests, test)

	test = &testCase{
		_id:            "pass-partial-tail",
		_buffer:        []byte("first\nsecond"),
		_expectedLines: []string{"first"},
		_expectedTail:  []byte("second"),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:            "pass-crlf-line",
		_buffer:        []byte("first\r\n"),
		_expectedLines: []string{"first"},
		_expectedTail:  nil,
	}
	tests = append(tests, test)

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			parser := &stubParser{}
			tail := parseCompleteLines(test._buffer, parser)
			assert.Equal(t, test._expectedLines, parser.lines)
			assert.Equal(t, test._expectedTail, tail)
		})
	}
}

func TestWatcherReadNewLines_AppendsCompleteLines(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("first\nsecond\n"), 0o644))

	w, parser := provideStubWatcher(path)
	w.readNewLines(path)

	assert.Equal(t, []string{"first", "second"}, parser.lines)
	assert.Equal(t, 1, parser.flushCount)
}

func TestWatcherReadNewLines_HoldsPartialLine(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("first\nsecond"), 0o644))

	w, parser := provideStubWatcher(path)
	w.readNewLines(path)

	assert.Equal(t, []string{"first"}, parser.lines)
	assert.Equal(t, []byte("second"), w.files[watchedFilePath(path)].partial)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString(" line\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	w.readNewLines(path)

	assert.Equal(t, []string{"first", "second line"}, parser.lines)
	assert.Nil(t, w.files[watchedFilePath(path)].partial)
	assert.Equal(t, 2, parser.flushCount)
}

func TestWatcherReadNewLines_DoesNotDuplicateParsedLines(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("first\n"), 0o644))

	w, parser := provideStubWatcher(path)
	w.readNewLines(path)
	w.readNewLines(path)

	assert.Equal(t, []string{"first"}, parser.lines)
	assert.Equal(t, int64(len([]byte("first\n"))), w.files[watchedFilePath(path)].offset)
}
