package session

type (
	Role string
)

var ValidRoles = []Role{
	RoleUser,
	RoleAssistant,
	RoleDeveloper,
}

const (
	RoleAssistant Role = "assistant"
	RoleDeveloper Role = "developer" // codex only
	RoleUser      Role = "user"
)

func (r Role) IsValid() bool {
	for _, role := range ValidRoles {
		if role == r {
			return true
		}
	}
	return false
}
