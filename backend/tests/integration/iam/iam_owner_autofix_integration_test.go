package iamintegration

import (
	"context"
	"testing"

	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
)

func TestIAMOwnerAutoFixIntegration(t *testing.T) {
	fx := setupIAMFixture(t)

	systemTenant := &modeltenant.Tenant{Key: "system", Name: "System", Status: modeltenant.TenantStatusActive, Type: modeltenant.TenantTypeSystem, Plan: modeltenant.TenantPlanFree}
	systemTenant.UUID = mustUUID("11111111-1111-4111-8111-111111111111")
	require.NoError(t, fx.DB.Create(systemTenant).Error)
	root := &modeliam.User{Email: "root-migration@example.com", DisplayName: "Root", Status: modeliam.UserStatusActive, IsRoot: true}
	require.NoError(t, fx.DB.Create(root).Error)
	rootMember := &modeliam.Member{TenantUUID: systemTenant.UUID.String(), UserID: root.ID, Username: "root", Status: modeliam.UserStatusActive}
	require.NoError(t, fx.DB.Create(rootMember).Error)

	tenant := &modeltenant.Tenant{Key: "legacy", Name: "Legacy", Status: modeltenant.TenantStatusActive, Type: modeltenant.TenantTypeEnterprise, Plan: modeltenant.TenantPlanFree}
	tenant.UUID = mustUUID("22222222-2222-4222-8222-222222222222")
	require.NoError(t, fx.DB.Create(tenant).Error)

	roleOwner := &modeliam.Role{Scope: string(coreiam.RoleScopeTenant), TenantUUID: tenant.UUID.String(), Code: coreiam.CodeRoleOwner, Name: "Tenant Owner", Builtin: true}
	roleAdmin := &modeliam.Role{Scope: string(coreiam.RoleScopeTenant), TenantUUID: tenant.UUID.String(), Code: coreiam.CodeRoleAdmin, Name: "Tenant Admin", Builtin: true}
	require.NoError(t, fx.DB.Create(roleOwner).Error)
	require.NoError(t, fx.DB.Create(roleAdmin).Error)

	adminUser := &modeliam.User{Email: "legacy-admin@example.com", DisplayName: "Legacy Admin", Status: modeliam.UserStatusActive}
	require.NoError(t, fx.DB.Create(adminUser).Error)
	adminMember := &modeliam.Member{TenantUUID: tenant.UUID.String(), UserID: adminUser.ID, Username: "legacy-admin", Status: modeliam.UserStatusActive}
	require.NoError(t, fx.DB.Create(adminMember).Error)
	require.NoError(t, fx.DB.Create(&modeliam.RoleBinding{
		TenantUUID:  tenant.UUID.String(),
		RoleID:      roleAdmin.ID,
		SubjectType: modeliam.SubMember,
		SubjectID:   adminMember.ID,
		DataScope:   modeliam.ScopeTenant,
	}).Error)

	ctx := reqctx.WithUserID(context.Background(), root.ID)
	out, err := iamsvc.NewIAMMigrationReportService(fx.DB).FixMissingOwners(ctx)
	require.NoError(t, err)
	require.Contains(t, out.FixedTenantUUIDs, tenant.UUID.String())

	var ownerBindings int64
	require.NoError(t, fx.DB.Table((&modeliam.RoleBinding{}).GetTableName(true)+" AS rb").
		Joins("JOIN "+(&modeliam.Role{}).GetTableName(true)+" AS r ON r.id = rb.role_id").
		Where("rb.tenant_uuid = ? AND rb.subject_id = ? AND r.code = ?", tenant.UUID.String(), adminMember.ID, coreiam.CodeRoleOwner).
		Count(&ownerBindings).Error)
	require.EqualValues(t, 1, ownerBindings)

	var auditEvents int64
	require.NoError(t, fx.DB.Model(&modelaudit.AuditEvent{}).
		Where("tenant_uuid = ? AND operation = ?", tenant.UUID.String(), "IAM_MIGRATION_FIX_OWNER").
		Count(&auditEvents).Error)
	require.EqualValues(t, 1, auditEvents)
}
