package seed

import (
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSeedGrantDefaultRolesForTenantGrantsAgentUseToUserOnly(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = prevSchema
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&dbm.Role{}, &dbm.Permission{}, &dbm.RolePermission{}))

	tenantUUID := "11111111-1111-1111-1111-111111111111"
	require.NoError(t, db.Create(&[]dbm.Role{
		{Scope: "tenant", TenantUUID: tenantUUID, Code: "role_admin", Name: "Tenant Admin", Builtin: true},
		{Scope: "tenant", TenantUUID: tenantUUID, Code: "role_user", Name: "Tenant User", Builtin: true},
		{Scope: "tenant", TenantUUID: tenantUUID, Code: "role_readonly", Name: "Tenant Readonly", Builtin: true},
		{Scope: "tenant", TenantUUID: tenantUUID, Code: "role_vendor", Name: "Vendor", Builtin: true},
	}).Error)
	require.NoError(t, db.Create(&[]dbm.Permission{
		{Module: "agent", Resource: "invoke", Action: "use", Effect: "allow", Status: dbm.PermissionStatusActive, Source: "platform_capability", Meta: []byte(`{"type":"action","module":"agent","title_i18n":{"zh-CN":"Agent 调用"}}`)},
		{Module: "agent", Resource: "session", Action: "manage", Effect: "allow", Status: dbm.PermissionStatusActive, Source: "platform_capability", Meta: []byte(`{"type":"action","module":"agent","title_i18n":{"zh-CN":"Agent 会话管理"}}`)},
		{Module: "iam", Resource: "permission", Action: "read", Effect: "allow", Status: dbm.PermissionStatusActive, Source: "core", Meta: []byte(`{"type":"action","module":"iam"}`)},
		{Module: "iam", Resource: "permission", Action: "cleanup", Effect: "allow", Status: dbm.PermissionStatusActive, Source: "platform_capability", Meta: []byte(`{"type":"action","module":"iam","title_i18n":{"zh-CN":"权限清理"}}`)},
		{Module: "menu", Resource: "agent", Action: "view", Effect: "allow", Status: dbm.PermissionStatusActive, Source: "core", Meta: []byte(`{"type":"menu","module":"menu"}`)},
	}).Error)

	require.NoError(t, SeedGrantDefaultRolesForTenant(db, tenantUUID))

	var user dbm.Role
	require.NoError(t, db.Where("code = ?", "role_user").First(&user).Error)
	var readonly dbm.Role
	require.NoError(t, db.Where("code = ?", "role_readonly").First(&readonly).Error)

	var userAgentUse int64
	require.NoError(t, db.Table("iam_role_permission rp").
		Joins("JOIN iam_permission p ON p.id = rp.permission_id").
		Where("rp.role_id = ? AND p.module = ? AND p.action IN ?", user.ID, "agent", []string{"use", "manage"}).
		Count(&userAgentUse).Error)
	require.Equal(t, int64(2), userAgentUse)

	var readonlyAgentUse int64
	require.NoError(t, db.Table("iam_role_permission rp").
		Joins("JOIN iam_permission p ON p.id = rp.permission_id").
		Where("rp.role_id = ? AND p.module = ? AND p.action IN ?", readonly.ID, "agent", []string{"use", "manage"}).
		Count(&readonlyAgentUse).Error)
	require.Equal(t, int64(0), readonlyAgentUse)
}
