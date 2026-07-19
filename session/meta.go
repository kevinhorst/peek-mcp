package session

type Meta struct {
	SessionId Id      `json:"session_id,omitempty"`
	CWD       string  `json:"cwd,omitempty"`
	GitBranch string  `json:"git_branch,omitempty"`
	Model     string  `json:"model,omitempty"`
	Origin    *Origin `json:"origin,omitempty"`
}

// Origin carries client/provenance metadata of the transcript that produced
// the session. Codex fills it from session_meta; Claude fills CliVersion.
type Origin struct {
	AgentNickname  string `json:"agent_nickname,omitempty"`
	CliVersion     string `json:"cli_version,omitempty"`
	CommitHash     string `json:"commit_hash,omitempty"`
	ForkedFromId   string `json:"forked_from_id,omitempty"`
	Originator     string `json:"originator,omitempty"`
	ParentThreadId string `json:"parent_thread_id,omitempty"`
	RepositoryUrl  string `json:"repository_url,omitempty"`
	SourceKind     string `json:"source_kind,omitempty"`
}

func (m *Meta) Update(other *Meta) {
	if other.SessionId != "" {
		m.SessionId = other.SessionId
	}

	if other.CWD != "" {
		m.CWD = other.CWD
	}

	if other.GitBranch != "" {
		m.GitBranch = other.GitBranch
	}

	if other.Model != "" {
		m.Model = other.Model
	}

	if other.Origin != nil {
		m.Origin = other.Origin
	}
}
