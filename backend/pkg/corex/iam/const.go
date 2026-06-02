package iam

type RoleScope string

const (
	RoleScopeSystem RoleScope = "system"
	RoleScopeTenant RoleScope = "tenant"
)

type RoleCode string

const (
	CodeSystemAdmin   RoleCode = "system_admin"
	CodeSystemMonitor RoleCode = "system_monitor"
	CodeRoleOwner     RoleCode = "role_owner"
	CodeRoleAdmin     RoleCode = "role_admin"
	CodeRoleUser      RoleCode = "role_user"
	CodeRoleReadonly  RoleCode = "role_readonly"

	SystemTenantID = uint64(0)
)
