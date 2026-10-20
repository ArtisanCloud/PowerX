package iam

import (
	"context"
	"time"

	authfacts "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/authorization"
	baseline "github.com/ArtisanCloud/PowerX/pkg/corex/iam/authorizationbaseline"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	base "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type PluginIdentityRepository struct {
	*base.BaseRepository[models.User]
	db *gorm.DB
}

func NewPluginIdentityRepository(db *gorm.DB) *PluginIdentityRepository {
	return &PluginIdentityRepository{BaseRepository: base.NewBaseRepository[models.User](db), db: db}
}
func (r *PluginIdentityRepository) User(ctx context.Context, id string) (models.User, error) {
	var row models.User
	err := r.db.WithContext(ctx).Where("uuid=?", id).First(&row).Error
	return row, err
}
func (r *PluginIdentityRepository) Tenant(ctx context.Context, id string) (tenant.Tenant, error) {
	var row tenant.Tenant
	err := r.db.WithContext(ctx).Where("uuid=?", id).First(&row).Error
	return row, err
}
func (r *PluginIdentityRepository) Member(ctx context.Context, tenantID, userID, id string) (models.Member, error) {
	var row models.Member
	err := r.db.WithContext(ctx).Where("uuid=? AND tenant_uuid=? AND user_uuid=?", id, tenantID, userID).First(&row).Error
	return row, err
}
func (r *PluginIdentityRepository) Session(ctx context.Context, jti string) (models.RefreshToken, error) {
	var row models.RefreshToken
	err := r.db.WithContext(ctx).Where("jti=?", jti).First(&row).Error
	return row, err
}

// Role UUIDs are resolved with the existing direct/dimension binding model.
// Numeric assignment IDs remain internal to that existing model; new contracts
// and permission relationships use user/member/role/permission UUIDs only.
func (r *PluginIdentityRepository) RoleUUIDs(ctx context.Context, tenantID, memberUUID string, memberID uint64, now time.Time) ([]string, error) {
	b := (&models.RoleBinding{}).TableName()
	a := (&models.MemberAssignment{}).TableName()
	roles := (&models.Role{}).TableName()
	ts := now.UnixMilli()
	sql := `SELECT DISTINCT rb.role_uuid FROM ` + b + ` rb JOIN ` + roles + ` role ON CAST(role.uuid AS TEXT)=rb.role_uuid AND role.deleted_at IS NULL AND (role.tenant_uuid=rb.tenant_uuid OR role.scope='system') WHERE rb.tenant_uuid=? AND ((rb.subject_type='MEMBER' AND rb.subject_uuid=?) OR EXISTS (SELECT 1 FROM ` + a + ` ma WHERE ma.tenant_uuid=rb.tenant_uuid AND ma.member_id=? AND rb.subject_id=ma.dim_id AND rb.subject_type=CASE ma.dim_type WHEN 'ORG' THEN 'ORG_UNIT' WHEN 'TEAM' THEN 'TEAM' WHEN 'POSITION' THEN 'POSITION' WHEN 'GROUP' THEN 'GROUP' END AND (ma.start_at IS NULL OR ma.start_at<=?) AND (ma.end_at IS NULL OR ma.end_at>?))) AND (rb.start_at IS NULL OR rb.start_at<=?) AND (rb.end_at IS NULL OR rb.end_at>?)`
	ids := []string{}
	err := r.db.WithContext(ctx).Raw(sql, tenantID, memberUUID, memberID, ts, ts, ts, ts).Scan(&ids).Error
	return ids, err
}
func (r *PluginIdentityRepository) Snapshot(ctx context.Context, tenantID, memberUUID string, memberID uint64, plugin string, root bool, now time.Time) ([]string, []models.Permission, error) {
	roleCodes := []string{}
	permissions := []models.Permission{}
	query := r.db.WithContext(ctx).Model(&models.Permission{}).Where("status=? AND effect=? AND source=?", models.PermissionStatusActive, "allow", "plugin:"+plugin)
	if root {
		roleCodes = []string{"role_system_admin"}
	} else {
		ids, err := r.RoleUUIDs(ctx, tenantID, memberUUID, memberID, now)
		if err != nil {
			return nil, nil, err
		}
		if len(ids) == 0 {
			return roleCodes, permissions, nil
		}
		if err = r.db.WithContext(ctx).Model(&models.Role{}).Where("uuid IN ?", ids).Order("code").Pluck("code", &roleCodes).Error; err != nil {
			return nil, nil, err
		}
		facts, err := authfacts.NewRepository(r.db).LoadPermissionFacts(ctx, tenantID, plugin)
		if err != nil {
			return nil, nil, err
		}
		_, permissions = baseline.SnapshotPermissions(facts, memberUUID, memberID, now)
		return roleCodes, permissions, nil
	}
	err := query.Find(&permissions).Error
	return roleCodes, permissions, err
}
