package bootstrap

import "fmt"

type Role string

const (
	RoleAll    Role = "all"
	RoleAPI    Role = "api"
	RoleWorker Role = "worker"
)

func ParseRole(value string) (Role, error) {
	role := Role(value)
	switch role {
	case RoleAll, RoleAPI, RoleWorker:
		return role, nil
	default:
		return "", fmt.Errorf("invalid runtime role %q: expected all, api, or worker", value)
	}
}

func (r Role) StartsAPI() bool {
	return r == RoleAll || r == RoleAPI
}

func (r Role) StartsWorker() bool {
	return r == RoleAll || r == RoleWorker
}
