package config

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/pkg/errors"
)

const (
	KeyBackLink              = "back-link"
	KeyDepth                 = "depth"
	KeyLogLevel              = "log-level"
	KeySnapshotRetentionDays = "snapshot-retention-days"
	KeyStateRetentionDays    = "state-retention-days"

	dirPerm  = 0o700
	filePerm = 0o600

	maxDepth         = 1000
	maxRetentionDays = 3650
	minDepth         = 1
)

var EditableKeys = []string{KeyBackLink, KeyDepth, KeyStateRetentionDays, KeySnapshotRetentionDays, KeyLogLevel}

var logLevels = []string{"debug", "info", "warn", "error"}

type File struct {
	BackLink              *string `json:"back_link,omitempty"`
	Depth                 *int    `json:"depth,omitempty"`
	LogLevel              *string `json:"log_level,omitempty"`
	SnapshotRetentionDays *int    `json:"snapshot_retention_days,omitempty"`
	StateRetentionDays    *int    `json:"state_retention_days,omitempty"`
}

func (f *File) Set(key, value string) error {
	switch key {
	case KeyBackLink:
		return f.setBackLink(value)
	case KeyDepth:
		return f.setDepth(value)
	case KeyLogLevel:
		return f.setLogLevel(value)
	case KeySnapshotRetentionDays:
		return f.setSnapshotRetentionDays(value)
	case KeyStateRetentionDays:
		return f.setStateRetentionDays(value)
	}
	return errors.Errorf("File.Set: Unknown or non-editable key: %s", key)
}

func (f *File) setBackLink(value string) error {
	if value == "" {
		f.BackLink = nil
		return nil
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.Errorf("File.setBackLink: Invalid field back-link: %s (want an absolute http(s) URL, empty hides the link)", value)
	}

	f.BackLink = &value
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
	if f.BackLink != nil {
		values[KeyBackLink] = *f.BackLink
	}
	if f.Depth != nil {
		values[KeyDepth] = strconv.Itoa(*f.Depth)
	}
	if f.LogLevel != nil {
		values[KeyLogLevel] = *f.LogLevel
	}
	if f.SnapshotRetentionDays != nil {
		values[KeySnapshotRetentionDays] = strconv.Itoa(*f.SnapshotRetentionDays)
	}
	if f.StateRetentionDays != nil {
		values[KeyStateRetentionDays] = strconv.Itoa(*f.StateRetentionDays)
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
