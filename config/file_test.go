package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	// missing-file-returns-empty
	t.Run("missing-file-returns-empty", func(t *testing.T) {
		file, err := Load(filepath.Join(t.TempDir(), "config.json"))
		require.NoError(t, err)
		assert.Equal(t, &File{}, file)
	})

	// invalid-json-errors
	t.Run("invalid-json-errors", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		require.NoError(t, os.WriteFile(path, []byte("{broken"), 0o600))

		_, err := Load(path)
		assert.Error(t, err)
	})

	// valid-file-roundtrip
	t.Run("valid-file-roundtrip", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		saved := &File{}
		require.NoError(t, saved.Set(KeyBackLink, "http://127.0.0.1:6001/"))
		require.NoError(t, saved.Set(KeyDepth, "50"))
		require.NoError(t, saved.Set(KeyLogLevel, "debug"))
		require.NoError(t, Save(path, saved))

		loaded, err := Load(path)
		require.NoError(t, err)
		assert.Equal(t, saved, loaded)
	})

	// retired-keys-ignored
	t.Run("retired-keys-ignored", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		content := `{"depth": 50, "poll_interval": "5s", "poll_window": "1h0m0s", "diff_cache_sessions": 25}`
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

		loaded, err := Load(path)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{KeyDepth: "50"}, loaded.FlagValues())
	})
}

func TestFile_Set(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool
		key         string
		value       string
	}

	tests := make([]*testCase, 0)

	// back-link-valid
	tests = append(tests, &testCase{
		_id:         "back-link-valid",
		_shouldPass: true,
		key:         KeyBackLink,
		value:       "http://127.0.0.1:6001/",
	})

	// back-link-not-a-url
	tests = append(tests, &testCase{
		_id:   "back-link-not-a-url",
		key:   KeyBackLink,
		value: "::bad",
	})

	// back-link-relative
	tests = append(tests, &testCase{
		_id:   "back-link-relative",
		key:   KeyBackLink,
		value: "/dashboard",
	})

	// back-link-bad-scheme
	tests = append(tests, &testCase{
		_id:   "back-link-bad-scheme",
		key:   KeyBackLink,
		value: "ftp://host/",
	})

	// depth-valid
	tests = append(tests, &testCase{
		_id:         "depth-valid",
		_shouldPass: true,
		key:         KeyDepth,
		value:       "50",
	})

	// depth-not-a-number
	tests = append(tests, &testCase{
		_id:   "depth-not-a-number",
		key:   KeyDepth,
		value: "abc",
	})

	// depth-out-of-range
	tests = append(tests, &testCase{
		_id:   "depth-out-of-range",
		key:   KeyDepth,
		value: "0",
	})

	// retired-key-poll-interval
	tests = append(tests, &testCase{
		_id:   "retired-key-poll-interval",
		key:   "poll-interval",
		value: "10s",
	})

	// log-level-valid
	tests = append(tests, &testCase{
		_id:         "log-level-valid",
		_shouldPass: true,
		key:         KeyLogLevel,
		value:       "warn",
	})

	// log-level-unknown
	tests = append(tests, &testCase{
		_id:   "log-level-unknown",
		key:   KeyLogLevel,
		value: "verbose",
	})

	// state-retention-zero-valid
	tests = append(tests, &testCase{
		_id:         "state-retention-zero-valid",
		_shouldPass: true,
		key:         KeyStateRetentionDays,
		value:       "0",
	})

	// state-retention-negative
	tests = append(tests, &testCase{
		_id:   "state-retention-negative",
		key:   KeyStateRetentionDays,
		value: "-1",
	})

	// unknown-key
	tests = append(tests, &testCase{
		_id:   "unknown-key",
		key:   "nonsense",
		value: "1",
	})

	// non-editable-key-transport
	tests = append(tests, &testCase{
		_id:   "non-editable-key-transport",
		key:   "transport",
		value: "stdio",
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			file := &File{}
			err := file.Set(test.key, test.value)
			assert.Equalf(t, test._shouldPass, err == nil, "err = %v", err)
			if test._shouldPass {
				assert.Equal(t, test.value, file.FlagValues()[test.key])
			}
		})
	}
}

func TestFile_SetBackLinkEmptyClears(t *testing.T) {
	file := &File{}
	require.NoError(t, file.Set(KeyBackLink, "http://127.0.0.1:6001/"))
	require.NotNil(t, file.BackLink)

	require.NoError(t, file.Set(KeyBackLink, ""))
	assert.Nil(t, file.BackLink)
	assert.NotContains(t, file.FlagValues(), KeyBackLink)
}

func TestSave(t *testing.T) {
	// roundtrip-load-equals-saved
	t.Run("roundtrip-load-equals-saved", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nested", "config.json")
		file := &File{}
		require.NoError(t, file.Set(KeyDepth, "50"))
		require.NoError(t, Save(path, file))

		loaded, err := Load(path)
		require.NoError(t, err)
		assert.Equal(t, file, loaded)
	})

	// creates-parent-dir
	t.Run("creates-parent-dir", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "peek", "config.json")
		require.NoError(t, Save(path, &File{}))

		info, err := os.Stat(path)
		require.NoError(t, err)
		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		}
	})
}
