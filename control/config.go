package control

import (
	"net/http"
	"strconv"

	"github.com/kevinhorst/peek-mcp/config"
)

const (
	tmplConfig    = "_config.html"
	tmplConfigRow = "_config_row.html"
)

type configData struct {
	RestartAvailable bool
	Rows             []configRow
}

type configRow struct {
	Editable      bool
	Explanation   string
	Key           string
	Overridden    bool
	RestartNeeded bool
	SavedValue    string
	Type          string
	Value         string
	Values        []string
}

func (s *Server) configRows() ([]configRow, error) {
	file, err := config.Load(s.configPath)
	if err != nil {
		return nil, err
	}

	saved := file.FlagValues()
	rows := make([]configRow, 0)
	rows = append(rows, readOnlyRow("MCP transport, fixed at launch", "transport", s.config.Transport))
	rows = append(rows, readOnlyRow("MCP HTTP port, fixed at launch", "port", strconv.Itoa(s.config.Port)))
	rows = append(rows, s.editableRow("turns kept per session (ring buffer)", config.KeyDepth, "number", strconv.Itoa(s.config.Depth), saved))
	rows = append(rows, readOnlyRow("Claude Code session root", "claude-home", s.config.ClaudeHome))
	rows = append(rows, readOnlyRow("Codex session root", "codex-home", s.config.CodexHome))
	rows = append(rows, readOnlyRow("Claude Desktop Cowork data root", "cowork-home", s.config.CoworkHome))
	rows = append(rows, s.editableRow("uncommitted-diff poll cadence", config.KeyPollInterval, "text", s.config.PollInterval, saved))
	rows = append(rows, s.editableRow("only poll repos active within this window", config.KeyPollWindow, "text", s.config.PollWindow, saved))
	rows = append(rows, readOnlyRow("diff/plan persistence root", "state-dir", s.config.StateDir))
	rows = append(rows, s.editableRow("days before idle session state is GCed, and startup ingest horizon (0 disables)", config.KeyStateRetentionDays, "number", strconv.Itoa(s.config.StateRetentionDays), saved))
	rows = append(rows, s.editableRow("days before diff snapshots are GCed (0 disables)", config.KeySnapshotRetentionDays, "number", strconv.Itoa(s.config.SnapshotRetentionDays), saved))
	rows = append(rows, s.editableRow("sessions whose diff snapshot stays cached in memory (0 disables)", config.KeyDiffCacheSessions, "number", strconv.Itoa(s.config.DiffCacheSessions), saved))
	rows = append(rows, readOnlyRow("dashboard port, fixed at launch", "control-port", strconv.Itoa(s.config.ControlPort)))
	rows = append(rows, readOnlyRow("bearer token protecting this dashboard", "control-token", tokenDisplay(s.config.TokenSet)))
	rows = append(rows, s.editableRow("URL the nav's Hub link points to (empty hides it)", config.KeyBackLink, "text", s.config.BackLink, saved))
	rows = append(rows, s.editableRow("slog level", config.KeyLogLevel, "enum", s.config.LogLevel, saved, "debug", "info", "warn", "error"))
	return rows, nil
}

func (s *Server) editableRow(explanation, key, rowType, runningValue string, savedValues map[string]string, values ...string) configRow {
	saved := savedValues[key]
	row := configRow{
		Editable:      true,
		Explanation:   explanation,
		Key:           key,
		Overridden:    s.overriddenKeys[key],
		RestartNeeded: saved != "" && saved != runningValue,
		SavedValue:    saved,
		Type:          rowType,
		Value:         runningValue,
		Values:        values,
	}
	if saved != "" {
		row.Value = saved
	}
	return row
}

func (s *Server) handleConfigFragment(w http.ResponseWriter, r *http.Request) {
	rows, err := s.configRows()
	if err != nil {
		respondInternalServerError(err, w)
		return
	}

	data := configData{RestartAvailable: s.restart != nil, Rows: rows}
	s.renderFragment(w, tmplConfig, data)
}

func (s *Server) handleConfigSet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	file, err := config.Load(s.configPath)
	if err != nil {
		respondInternalServerError(err, w)
		return
	}

	if err := file.Set(key, r.FormValue("value")); err != nil {
		respondBadRequest(err.Error(), w)
		return
	}

	if err := config.Save(s.configPath, file); err != nil {
		respondInternalServerError(err, w)
		return
	}

	w.Header().Set("HX-Trigger", "config-op")
	s.renderConfigRow(key, w)
}

func (s *Server) renderConfigRow(key string, w http.ResponseWriter) {
	rows, err := s.configRows()
	if err != nil {
		respondInternalServerError(err, w)
		return
	}

	for _, row := range rows {
		if row.Key == key {
			s.renderFragment(w, tmplConfigRow, row)
			return
		}
	}

	respondNotFound("unknown config key", w)
}

func readOnlyRow(explanation, key, value string) configRow {
	return configRow{Explanation: explanation, Key: key, Value: value}
}

func tokenDisplay(isSet bool) string {
	if isSet {
		return "set"
	}
	return "not set"
}
