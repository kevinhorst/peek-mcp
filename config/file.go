package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

const (
	KeyDepth                 = "depth"
	KeyDiffCacheSessions     = "diff-cache-sessions"
	KeyLogLevel              = "log-level"
	KeyPollInterval          = "poll-interval"
	KeyPollWindow            = "poll-window"
	KeySnapshotRetentionDays = "snapshot-retention-days"
	KeyStateRetentionDays    = "state-retention-days"

	dirPerm  = 0o700
	filePerm = 0o600

	maxDepth             = 1000
	maxDiffCacheSessions = 1000
	maxRetentionDays     = 3650
	minDepth             = 1
	minPollInterval      = time.Second
	minPollWindow        = time.Minute
)

var EditableKeys = []string{KeyDepth, KeyPollInterval, KeyPollWindow, KeyStateRetentionDays, KeySnapshotRetentionDays, KeyDiffCacheSessions, KeyLogLevel}

var logLevels = []string{"debug", "info", "warn", "error"}

type File struct {
	Depth                 *int    `json:"depth,omitempty"`
	DiffCacheSessions     *int    `json:"diff_cache_sessions,omitempty"`
	LogLevel              *string `json:"log_level,omitempty"`
	PollInterval          *string `json:"poll_interval,omitempty"`
	PollWindow            *string `json:"poll_window,omitempty"`
	SnapshotRetentionDays *int    `json:"snapshot_retention_days,omitempty"`
	StateRetentionDays    *int    `json:"state_retention_days,omitempty"`
}

func (f *File) Set(key, value string) error {
	switch key {
	case KeyDepth:
		return f.setDepth(value)
	case KeyDiffCacheSessions:
		return f.setDiffCacheSessions(value)
	case KeyLogLevel:
		return f.setLogLevel(value)
	case KeyPollInterval:
		return f.setPollInterval(value)
	case KeyPollWindow:
		return f.setPollWindow(value)
	case KeySnapshotRetentionDays:
		return f.setSnapshotRetentionDays(value)
	case KeyStateRetentionDays:
		return f.setStateRetentionDays(value)
	}
	return errors.Errorf("File.Set: Unknown or non-editable key: %s", key)
}

func (f *File) setDiffCacheSessions(value string) error {
	sessions, err := strconv.Atoi(value)
	if err != nil {
		return errors.Errorf("File.setDiffCacheSessions: Invalid field diff-cache-sessions: %s (want an integer)", value)
	}
	if sessions < 0 || sessions > maxDiffCacheSessions {
		return errors.Errorf("File.setDiffCacheSessions: Invalid field diff-cache-sessions: %d (want 0-%d, 0 disables caching)", sessions, maxDiffCacheSessions)
	}

	f.DiffCacheSessions = &sessions
	return nil
}

func (f *File) setSnapshotRetentionDays(value string) error {
	days, err := strconv.Atoi(value)
	if err != nil {
		return errors.Errorf("File.setSnapshotRetentionDays: Invalid field snapshot-retention-days: %s (want an integer)", value)
	}
	if days < 0 || days > maxRetentionDays {
		return errors.Errorf("File.setSnapshotRetentionDays: Invalid field snapshot-retention-days: %d (want 0-%d, 0 disables snapshot GC)", days, maxRetentionDays)
	}

	f.SnapshotRetentionDays = &days
	return nil
}

func (f *File) setDepth(value string) error {
	depth, err := strconv.Atoi(value)
	if err != nil {
		return errors.Errorf("File.setDepth: Invalid field depth: %s (want an integer)", value)
	}
	if depth < minDepth || depth > maxDepth {
		return errors.Errorf("File.setDepth: Invalid field depth: %d (want %d-%d)", depth, minDepth, maxDepth)
	}

	f.Depth = &depth
	return nil
}

func (f *File) setLogLevel(value string) error {
	if !slices.Contains(logLevels, value) {
		return errors.Errorf("File.setLogLevel: Invalid field log-level: %s (want debug|info|warn|error)", value)
	}

	f.LogLevel = &value
	return nil
}

func (f *File) setPollInterval(value string) error {
	interval, err := time.ParseDuration(value)
	if err != nil {
		return errors.Errorf("File.setPollInterval: Invalid field poll-interval: %s (want a Go duration, e.g. 5s)", value)
	}
	if interval < minPollInterval {
		return errors.Errorf("File.setPollInterval: Invalid field poll-interval: %s (want >= 1s)", value)
	}

	normalized := interval.String()
	f.PollInterval = &normalized
	return nil
}

func (f *File) setPollWindow(value string) error {
	window, err := time.ParseDuration(value)
	if err != nil {
		return errors.Errorf("File.setPollWindow: Invalid field poll-window: %s (want a Go duration, e.g. 1h)", value)
	}
	if window < minPollWindow {
		return errors.Errorf("File.setPollWindow: Invalid field poll-window: %s (want >= 1m)", value)
	}

	normalized := window.String()
	f.PollWindow = &normalized
	return nil
}

func (f *File) setStateRetentionDays(value string) error {
	days, err := strconv.Atoi(value)
	if err != nil {
		return errors.Errorf("File.setStateRetentionDays: Invalid field state-retention-days: %s (want an integer)", value)
	}
	if days < 0 || days > maxRetentionDays {
		return errors.Errorf("File.setStateRetentionDays: Invalid field state-retention-days: %d (want 0-%d, 0 disables GC)", days, maxRetentionDays)
	}

	f.StateRetentionDays = &days
	return nil
}

func (f *File) FlagValues() map[string]string {
	values := make(map[string]string)
	if f.Depth != nil {
		values[KeyDepth] = strconv.Itoa(*f.Depth)
	}
	if f.LogLevel != nil {
		values[KeyLogLevel] = *f.LogLevel
	}
	if f.PollInterval != nil {
		values[KeyPollInterval] = *f.PollInterval
	}
	if f.PollWindow != nil {
		values[KeyPollWindow] = *f.PollWindow
	}
	if f.SnapshotRetentionDays != nil {
		values[KeySnapshotRetentionDays] = strconv.Itoa(*f.SnapshotRetentionDays)
	}
	if f.StateRetentionDays != nil {
		values[KeyStateRetentionDays] = strconv.Itoa(*f.StateRetentionDays)
	}
	if f.DiffCacheSessions != nil {
		values[KeyDiffCacheSessions] = strconv.Itoa(*f.DiffCacheSessions)
	}
	return values
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", ".peek", "config.json")
	}
	return filepath.Join(home, ".peek", "config.json")
}

func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &File{}, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "Load: Failed to read config file")
	}

	file := &File{}
	if err := json.Unmarshal(data, file); err != nil {
		return nil, errors.Wrapf(err, "Load: Invalid config file %s", path)
	}
	return file, nil
}

func Save(path string, file *File) error {
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return errors.Wrap(err, "Save: Failed to marshal config")
	}

	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return errors.Wrap(err, "Save: Failed to create config directory")
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, filePerm); err != nil {
		return errors.Wrap(err, "Save: Failed to write temp file")
	}

	if err := os.Rename(tmp, path); err != nil {
		return errors.Wrap(err, "Save: Failed to rename temp file")
	}
	return nil
}
