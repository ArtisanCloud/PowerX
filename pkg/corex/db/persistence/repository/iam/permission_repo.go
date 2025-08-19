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
		Joins("JOIN "+(&dbm.UserRole{}).GetTableName(true)+" ur ON ur.role_id = rp.role_id AND ur.tenant_id = ?", tenantID).
		Where("ur.user_id = ? AND p.resource = ? AND p.action = ?", userID, resource, action).
		Count(&cnt).Error
	return cnt > 0, err
}
