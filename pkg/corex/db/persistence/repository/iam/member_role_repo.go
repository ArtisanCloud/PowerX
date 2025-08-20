package iam

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// MemberRoleRepository：用户与角色的绑定/查询
type MemberRoleRepository struct {
	*repository.BaseRepository[dbm.MemberRole]
	db *gorm.DB
}

func NewMemberRoleRepository(db *gorm.DB) *MemberRoleRepository {
	return &MemberRoleRepository{
		BaseRepository: repository.NewBaseRepository[dbm.MemberRole](db),
		db:             db,
	}
}

// ListMemberRoles 返回用户的角色列表
func (r *MemberRoleRepository) ListMemberRoles(ctx context.Context, tenantID, userID uint64) ([]dbm.Role, error) {
	var roles []dbm.Role
	err := r.db.WithContext(ctx).
		Table((&dbm.Role{}).GetTableName(true)+" AS r").
		Select("r.*").
		Joins("JOIN "+(&dbm.MemberRole{}).GetTableName(true)+" ur ON ur.role_id = r.id AND ur.tenant_id = ?", tenantID).
		Where("ur.user_id = ?", userID).
		Find(&roles).Error
	return roles, err
}

func (r *MemberRoleRepository) AssignRolesByCodes(ctx context.Context, tenantID, memberID uint64, codes ...string) error {
	if len(codes) == 0 {
		return nil
	}
	// 查出目标角色
	var roles []dbm.Role
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND code IN ?", tenantID, codes).
		Find(&roles).Error; err != nil {
		return err
	}
	// 逐个 upsert（member_id, role_id 主键幂等）
	for _, role := range roles {
		mr := &dbm.MemberRole{
			MemberID: memberID,
			RoleID:   role.ID,
			TenantID: tenantID,
		}
		if err := r.db.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(mr).Error; err != nil {
			return err
		}
	}
	return nil
}

// 列出某成员在租户下的角色
func (r *MemberRoleRepository) ListRolesByMember(ctx context.Context, tenantID, memberID uint64) ([]dbm.Role, error) {
	var roles []dbm.Role
	if err := r.db.WithContext(ctx).
		Table((&dbm.Role{}).TableName()+" AS r").
		Joins("JOIN "+(&dbm.MemberRole{}).TableName()+" AS mr ON mr.role_id = r.id").
		Where("mr.tenant_id = ? AND mr.member_id = ?", tenantID, memberID).
		Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}
