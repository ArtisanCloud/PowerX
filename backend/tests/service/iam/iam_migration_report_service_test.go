package iamservice_test

import (
	"context"
	"fmt"
	"testing"

	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	modelbase "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modelsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestIAMMigrationReportClassifiesOwnerAndAdminGaps(t *testing.T) {
	db := setupMigrationReportDB(t)
	ctx := context.Background()

	systemTenant := insertTenant(t, db, "system", modeltenant.TenantTypeSystem)
	root := insertUser(t, db, "root@example.com", true)
	insertMember(t, db, systemTenant.UUID.String(), root.ID, "root", modeliam.UserStatusActive)

	ownerMissing := insertTenant(t, db, "owner-missing", modeltenant.TenantTypeEnterprise)
	adminMember := insertMember(t, db, ownerMissing.UUID.String(), insertUser(t, db, "admin@example.com", false).ID, "admin", modeliam.UserStatusActive)
	ensureDefaultRoles(t, db, ownerMissing.UUID.String())
	bindRole(t, db, ownerMissing.UUID.String(), adminMember.ID, coreiam.CodeRoleAdmin)

	adminMissing := insertTenant(t, db, "admin-missing", modeltenant.TenantTypeEnterprise)
	ensureDefaultRoles(t, db, adminMissing.UUID.String())

	healthy := insertTenant(t, db, "healthy", modeltenant.TenantTypeEnterprise)
	healthyMember := insertMember(t, db, healthy.UUID.String(), insertUser(t, db, "owner@example.com", false).ID, "owner", modeliam.UserStatusActive)
	ensureDefaultRoles(t, db, healthy.UUID.String())
	bindRole(t, db, healthy.UUID.String(), healthyMember.ID, coreiam.CodeRoleOwner)
	bindRole(t, db, healthy.UUID.String(), healthyMember.ID, coreiam.CodeRoleAdmin)

	report, err := iamsvc.NewIAMMigrationReportService(db).Report(ctx)
	require.NoError(t, err)

	require.Equal(t, iamsvc.MigrationStatusOK, report.SystemTenantStatus)
	require.Equal(t, iamsvc.MigrationStatusOK, report.RootSystemMemberStatus)
	require.Len(t, report.RootUsers, 1)
	require.Contains(t, report.TenantOwnerMissing, ownerMissing.UUID.String())
	require.Contains(t, report.AutoFixCandidates, ownerMissing.UUID.String())
	require.Contains(t, report.TenantAdminMissing, adminMissing.UUID.String())
	require.Contains(t, report.ManualFixRequired, adminMissing.UUID.String())
	require.NotContains(t, report.TenantOwnerMissing, healthy.UUID.String())
}

func TestIAMMigrationFixOwnersOnlyFixesTenantsWithActiveAdmin(t *testing.T) {
	db := setupMigrationReportDB(t)
	ctx := context.Background()

	systemTenant := insertTenant(t, db, "system", modeltenant.TenantTypeSystem)
	root := insertUser(t, db, "root@example.com", true)
	insertMember(t, db, systemTenant.UUID.String(), root.ID, "root", modeliam.UserStatusActive)
	ctx = reqctx.WithUserID(ctx, root.ID)

	ownerMissing := insertTenant(t, db, "owner-missing", modeltenant.TenantTypeEnterprise)
	adminMember := insertMember(t, db, ownerMissing.UUID.String(), insertUser(t, db, "admin@example.com", false).ID, "admin", modeliam.UserStatusActive)
	ensureDefaultRoles(t, db, ownerMissing.UUID.String())
	bindRole(t, db, ownerMissing.UUID.String(), adminMember.ID, coreiam.CodeRoleAdmin)

	adminMissing := insertTenant(t, db, "admin-missing", modeltenant.TenantTypeEnterprise)
	ensureDefaultRoles(t, db, adminMissing.UUID.String())

	result, err := iamsvc.NewIAMMigrationReportService(db).FixMissingOwners(ctx)
	require.NoError(t, err)
	require.Contains(t, result.FixedTenantUUIDs, ownerMissing.UUID.String())
	require.NotContains(t, result.FixedTenantUUIDs, adminMissing.UUID.String())
	require.Contains(t, result.Report.ManualFixRequired, adminMissing.UUID.String())

	require.True(t, memberHasRole(t, db, ownerMissing.UUID.String(), adminMember.ID, coreiam.CodeRoleOwner))
	require.False(t, tenantHasRoleBinding(t, db, adminMissing.UUID.String(), coreiam.CodeRoleOwner))

	var auditCount int64
	require.NoError(t, db.Model(&modelaudit.AuditEvent{}).
		Where("operation = ? AND resource_type = ?", "IAM_MIGRATION_FIX_OWNER", "tenant").
		Count(&auditCount).Error)
	require.EqualValues(t, 1, auditCount)
}

func setupMigrationReportDB(t *testing.T) *gorm.DB {
	t.Helper()
	prevSchema := modelbase.PowerXSchema
	modelbase.PowerXSchema = "main"
	t.Cleanup(func() { modelbase.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared&_loc=UTC", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&modeltenant.Tenant{},
		&modeliam.User{},
		&modeliam.Member{},
		&modeliam.Role{},
		&modeliam.RoleBinding{},
		&modelsetting.SystemSetting{},
		&modelaudit.AuditEvent{},
	))
	return db
}

func insertTenant(t *testing.T, db *gorm.DB, key, tenantType string) *modeltenant.Tenant {
	t.Helper()
	item := &modeltenant.Tenant{
		Key:    key,
		Name:   key,
		Status: modeltenant.TenantStatusActive,
		Type:   tenantType,
		Plan:   modeltenant.TenantPlanFree,
	}
	require.NoError(t, db.Create(item).Error)
	return item
}

func insertUser(t *testing.T, db *gorm.DB, email string, isRoot bool) *modeliam.User {
	t.Helper()
	item := &modeliam.User{Email: email, DisplayName: email, Status: modeliam.UserStatusActive, IsRoot: isRoot}
	item.UUID = uuid.New()
	require.NoError(t, db.Create(item).Error)
	return item
}

func insertMember(t *testing.T, db *gorm.DB, tenantUUID string, userID uint64, username string, status int16) *modeliam.Member {
	t.Helper()
	item := &modeliam.Member{TenantUUID: tenantUUID, UserID: userID, Username: username, DisplayName: username, Status: status}
	item.UUID = uuid.New()
	require.NoError(t, db.Create(item).Error)
	return item
}

func ensureDefaultRoles(t *testing.T, db *gorm.DB, tenantUUID string) {
	t.Helper()
	for _, role := range []struct {
		code coreiam.RoleCode
		name string
	}{
		{coreiam.CodeRoleOwner, "Tenant Owner"},
		{coreiam.CodeRoleAdmin, "Tenant Admin"},
		{coreiam.CodeRoleUser, "Tenant User"},
	} {
		require.NoError(t, db.Create(&modeliam.Role{
			Scope:      string(coreiam.RoleScopeTenant),
			TenantUUID: tenantUUID,
			Code:       role.code,
			Name:       role.name,
			Builtin:    true,
		}).Error)
	}
}

func bindRole(t *testing.T, db *gorm.DB, tenantUUID string, memberID uint64, code coreiam.RoleCode) {
	t.Helper()
	var role modeliam.Role
	require.NoError(t, db.Where("tenant_uuid = ? AND code = ?", tenantUUID, code).First(&role).Error)
	require.NoError(t, db.Create(&modeliam.RoleBinding{
		TenantUUID:  tenantUUID,
		RoleID:      role.ID,
		SubjectType: modeliam.SubMember,
		SubjectID:   memberID,
		DataScope:   modeliam.ScopeTenant,
	}).Error)
}

func memberHasRole(t *testing.T, db *gorm.DB, tenantUUID string, memberID uint64, code coreiam.RoleCode) bool {
	t.Helper()
	var count int64
	require.NoError(t, db.Table((&modeliam.RoleBinding{}).GetTableName(true)+" AS rb").
		Joins("JOIN "+(&modeliam.Role{}).GetTableName(true)+" AS r ON r.id = rb.role_id").
		Where("rb.tenant_uuid = ? AND rb.subject_type = ? AND rb.subject_id = ? AND r.code = ?", tenantUUID, modeliam.SubMember, memberID, code).
		Count(&count).Error)
	return count > 0
}

func tenantHasRoleBinding(t *testing.T, db *gorm.DB, tenantUUID string, code coreiam.RoleCode) bool {
	t.Helper()
	var count int64
	require.NoError(t, db.Table((&modeliam.RoleBinding{}).GetTableName(true)+" AS rb").
		Joins("JOIN "+(&modeliam.Role{}).GetTableName(true)+" AS r ON r.id = rb.role_id").
		Where("rb.tenant_uuid = ? AND r.code = ?", tenantUUID, code).
		Count(&count).Error)
	return count > 0
}
