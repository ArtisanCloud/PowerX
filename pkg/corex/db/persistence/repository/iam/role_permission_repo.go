package iam

import (
	"context"

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
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

// ListRolePermissions 查询角色拥有的权限列表
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
