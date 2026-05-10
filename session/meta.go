package session

type Meta struct {
	SessionId Id     `json:"session_id,omitempty"`
	CWD       string `json:"cwd,omitempty"`
	GitBranch string `json:"git_branch,omitempty"`
	Model     string `json:"model,omitempty"`
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
}
