package iam

type RoleScope string

const (
	RoleScopeSystem RoleScope = "system"
	RoleScopeTenant RoleScope = "tenant"
)

const (
	CodeSystemAdmin   = "system_admin"
	CodeSystemMonitor = "system_monitor"
	CodeRoleAdmin     = "role_admin"
	CodeRoleUser      = "role_user"

	SystemTenantID = uint64(0)
)
