package iam

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// RoleRepository：基础 CRUD + 常用查询
type RoleRepository struct {
	*repository.BaseRepository[dbm.Role]
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{
		BaseRepository: repository.NewBaseRepository[dbm.Role](db),
		db:             db,
	}
}

// FindByCode 在 scope+tenant 上按 code 查找
func (r *RoleRepository) FindByCode(ctx context.Context, scope string, tenantID *uint64, code string) (*dbm.Role, error) {
	var role dbm.Role
	q := r.db.WithContext(ctx).Where("scope = ? AND code = ?", scope, code)
	if scope == "tenant" && tenantID != nil {
		q = q.Where("tenant_id = ?", *tenantID)
	} else {
		q = q.Where("tenant_id IS NULL")
	}
	if err := q.First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// EnsureDefaultRoles 为租户确保默认角色（示例：role_admin、role_user）
func (r *RoleRepository) EnsureDefaultRoles(ctx context.Context, tenantID uint64) error {
	defs := []dbm.Role{
		{Scope: "tenant", TenantID: tenantID, Code: "role_admin", Name: "Tenant Admin", Builtin: true},
		{Scope: "tenant", TenantID: tenantID, Code: "role_user", Name: "Tenant User", Builtin: true},
	}
	for i := range defs {
		if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "scope"}, {Name: "tenant_id"}, {Name: "code"}},
			DoNothing: true,
		}).Create(&defs[i]).Error; err != nil {
			return err
		}
	}
	return nil
}
