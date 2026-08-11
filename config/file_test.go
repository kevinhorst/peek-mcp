package config

import (
	"os"
	"path/filepath"
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
		require.NoError(t, saved.Set(KeyDepth, "50"))
		require.NoError(t, saved.Set(KeyLogLevel, "debug"))
		require.NoError(t, Save(path, saved))

		loaded, err := Load(path)
		require.NoError(t, err)
		assert.Equal(t, saved, loaded)
	})
}

func TestFile_Set(t *testing.T) {
	type testCase struct {
		_id         string
		_saved      string
		_shouldPass bool
		key         string
		value       string
	}

	tests := make([]*testCase, 0)

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

	// poll-interval-valid-normalized
	tests = append(tests, &testCase{
		_id:         "poll-interval-valid-normalized",
		_shouldPass: true,
		key:         KeyPollInterval,
		value:       "10s",
	})

	// poll-interval-below-minimum
	tests = append(tests, &testCase{
		_id:   "poll-interval-below-minimum",
		key:   KeyPollInterval,
		value: "500ms",
	})

	// poll-window-valid
	tests = append(tests, &testCase{
		_id:         "poll-window-valid",
		_saved:      "1h0m0s",
		_shouldPass: true,
		key:         KeyPollWindow,
		value:       "1h",
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
				saved := test._saved
				if saved == "" {
					saved = test.value
				}
				assert.Equal(t, saved, file.FlagValues()[test.key])
			}
		})
	}
}

func TestSave(t *testing.T) {
	// roundtrip-load-equals-saved
	t.Run("roundtrip-load-equals-saved", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nested", "config.json")
		file := &File{}
		require.NoError(t, file.Set(KeyPollInterval, "10s"))
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
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})
}
