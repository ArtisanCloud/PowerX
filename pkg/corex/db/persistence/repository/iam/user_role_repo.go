package iam

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// UserRoleRepository：用户与角色的绑定/查询
type UserRoleRepository struct {
	*repository.BaseRepository[dbm.UserRole]
	db *gorm.DB
}

func NewUserRoleRepository(db *gorm.DB) *UserRoleRepository {
	return &UserRoleRepository{
		BaseRepository: repository.NewBaseRepository[dbm.UserRole](db),
		db:             db,
	}
}

// AssignRolesByCodes 按角色 code 绑定（传入已查好的 Role 列表更高效，这里提供便捷接口）
func (r *UserRoleRepository) AssignRolesByCodes(ctx context.Context, tenantID, userID uint64, roleCodes ...string) error {
	if len(roleCodes) == 0 {
		return nil
	}
	// 查 Role
	var roles []dbm.Role
	if err := r.db.WithContext(ctx).
		Where("code IN ?", roleCodes).
		Find(&roles).Error; err != nil {
		return err
	}
	// 绑定（幂等）
	rows := make([]dbm.UserRole, 0, len(roles))
	for _, role := range roles {
		rows = append(rows, dbm.UserRole{
			UserID:   userID,
			RoleID:   role.ID,
			TenantID: tenantID,
		})
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

// ListUserRoles 返回用户的角色列表
func (r *UserRoleRepository) ListUserRoles(ctx context.Context, tenantID, userID uint64) ([]dbm.Role, error) {
	var roles []dbm.Role
	err := r.db.WithContext(ctx).
		Table((&dbm.Role{}).GetTableName(true)+" AS r").
		Select("r.*").
		Joins("JOIN "+(&dbm.UserRole{}).GetTableName(true)+" ur ON ur.role_id = r.id AND ur.tenant_id = ?", tenantID).
		Where("ur.user_id = ?", userID).
		Find(&roles).Error
	return roles, err
}
