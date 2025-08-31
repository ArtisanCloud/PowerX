// pkg/corex/db/persistence/repository/iam/role_permission_repo.go
package iam

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// RolePermissionRepository：给角色绑定/查询权限
type RolePermissionRepository struct {
	*repository.BaseRepository[dbm.RolePermission]
	db *gorm.DB
}

func NewRolePermissionRepository(db *gorm.DB) *RolePermissionRepository {
	return &RolePermissionRepository{
		BaseRepository: repository.NewBaseRepository[dbm.RolePermission](db),
		db:             db,
	}
}

// BindPermissions 角色绑定多个权限（幂等）
func (r *RolePermissionRepository) BindPermissions(ctx context.Context, roleID uint64, permIDs ...uint64) error {
	if len(permIDs) == 0 {
		return nil
	}
	rows := make([]dbm.RolePermission, 0, len(permIDs))
	for _, pid := range permIDs {
		rows = append(rows, dbm.RolePermission{RoleID: roleID, PermissionID: pid})
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&rows).Error
}

// ListRolePermissions 查询角色拥有的权限详情（Permission 列表）
func (r *RolePermissionRepository) ListRolePermissions(ctx context.Context, roleID uint64) ([]dbm.Permission, error) {
	var perms []dbm.Permission
	err := r.db.WithContext(ctx).
		Table((&dbm.Permission{}).GetTableName(true)+" AS p").
		Select("p.*").
		Joins("JOIN "+(&dbm.RolePermission{}).GetTableName(true)+" rp ON rp.permission_id = p.id").
		Where("rp.role_id = ?", roleID).
		Find(&perms).Error
	return perms, err
}

// ✅ 新增：只取 Permission ID 列表（给 SetPermissionIDs 这种增量接口用）
func (r *RolePermissionRepository) ListPermissionIDsOfRole(ctx context.Context, roleID uint64) ([]uint64, error) {
	var ids []uint64
	err := r.db.WithContext(ctx).
		Table((&dbm.RolePermission{}).GetTableName(true)).
		Where("role_id = ?", roleID).
		Pluck("permission_id", &ids).Error
	return ids, err
}

// ✅ 修复：使用 GORM 原生 OnConflict
func (r *RolePermissionRepository) GrantByIDsTx(tx *gorm.DB, roleID uint64, ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	rows := make([]dbm.RolePermission, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, dbm.RolePermission{
			RoleID:       roleID,
			PermissionID: id,
			CreatedAt:    now, // 或留 0 让 autoCreateTime 生效
		})
	}
	return tx.Table((&dbm.RolePermission{}).GetTableName(true)).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&rows).Error
}

func (r *RolePermissionRepository) RevokeByIDsTx(tx *gorm.DB, roleID uint64, ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	return tx.WithContext(tx.Statement.Context).
		Table((&dbm.RolePermission{}).GetTableName(true)).
		Where("role_id = ? AND permission_id IN ?", roleID, ids).
		Delete(nil).Error
}
