package iam

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// PermissionRepository：基础 CRUD + 批量 Upsert + 用户权限检查
type PermissionRepository struct {
	*repository.BaseRepository[dbm.Permission]
	db *gorm.DB
}

func NewPermissionRepository(db *gorm.DB) *PermissionRepository {
	return &PermissionRepository{
		BaseRepository: repository.NewBaseRepository[dbm.Permission](db),
		db:             db,
	}
}

// UpsertBatch 按 (plugin, resource, action) 幂等写入
func (r *PermissionRepository) UpsertBatch(ctx context.Context, perms []dbm.Permission) error {
	if len(perms) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "plugin"}, {Name: "resource"}, {Name: "action"}},
		DoNothing: true,
	}).Create(&perms).Error
}

// UserHasPermission 判断用户在租户下是否拥有某资源-动作
func (r *PermissionRepository) UserHasPermission(ctx context.Context, tenantID, userID uint64, resource, action string) (bool, error) {
	var cnt int64
	err := r.db.WithContext(ctx).
		Table((&dbm.Permission{}).GetTableName(true)+" AS p").
		Select("COUNT(1)").
		Joins("JOIN "+(&dbm.RolePermission{}).GetTableName(true)+" rp ON rp.permission_id = p.id").
		Joins("JOIN "+(&dbm.MemberRole{}).GetTableName(true)+" ur ON ur.role_id = rp.role_id AND ur.tenant_id = ?", tenantID).
		Where("ur.user_id = ? AND p.resource = ? AND p.action = ?", userID, resource, action).
		Count(&cnt).Error
	return cnt > 0, err
}

// 在 pkg/corex/db/persistence/repository/iam/permission_repo.go 里补充：
// PermissionRepository.MemberHasPermissionViaBinding ——用 EXISTS 更高效，也没有 t 变量
func (r *PermissionRepository) MemberHasPermissionViaBinding(
	ctx context.Context, tenantID, memberID uint64, resource, action string,
) (bool, error) {
	tP := (&dbm.Permission{}).GetTableName(true)
	tRP := (&dbm.RolePermission{}).GetTableName(true)
	tRB := (&dbm.RoleBinding{}).GetTableName(true)
	tMA := (&dbm.MemberAssignment{}).GetTableName(true)

	var ok bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT
		  EXISTS (
		    SELECT 1
		      FROM `+tP+`  p
		      JOIN `+tRP+` rp ON rp.permission_id = p.id
		      JOIN `+tRB+` rb ON rb.role_id = rp.role_id AND rb.tenant_id = ?
		     WHERE rb.subject_type = ? AND rb.subject_id = ? AND p.resource = ? AND p.action = ?
		  )
		  OR
		  EXISTS (
		    SELECT 1
		      FROM `+tP+`  p
		      JOIN `+tRP+` rp ON rp.permission_id = p.id
		      JOIN `+tRB+` rb ON rb.role_id = rp.role_id AND rb.tenant_id = ?
		      JOIN `+tMA+` ma ON ma.tenant_id = rb.tenant_id
		                     AND rb.subject_type = ma.dim_type
		                     AND rb.subject_id   = ma.dim_id
		     WHERE ma.member_id = ? AND p.resource = ? AND p.action = ?
		  )`,
		tenantID, dbm.SubMember, memberID, resource, action,
		tenantID, memberID, resource, action,
	).Scan(&ok).Error
	return ok, err
}
