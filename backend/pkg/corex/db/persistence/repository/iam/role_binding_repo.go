// pkg/corex/db/persistence/repository/iam/role_binding_repo.go
package iam

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type RoleBindingRepository struct {
	*repository.BaseRepository[dbm.RoleBinding]
	db *gorm.DB
}

func NewRoleBindingRepository(db *gorm.DB) *RoleBindingRepository {
	return &RoleBindingRepository{
		BaseRepository: repository.NewBaseRepository[dbm.RoleBinding](db),
		db:             db,
	}
}

// 幂等创建绑定（建议你在表上建唯一键：tenant_id,role_id,subject_type,subject_id）
func (r *RoleBindingRepository) Create(ctx context.Context, rb *dbm.RoleBinding) error {
	if rb == nil || rb.TenantID == 0 || rb.RoleID == 0 || rb.SubjectID == 0 || rb.SubjectType == "" {
		return errors.New("invalid role binding")
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(rb).Error
}

func (r *RoleBindingRepository) Delete(ctx context.Context, tenantID, id uint64) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Delete(&dbm.RoleBinding{}).Error
}

func (r *RoleBindingRepository) ListBySubject(ctx context.Context, tenantID uint64, st dbm.SubjectType, sid uint64) ([]dbm.RoleBinding, error) {
	var out []dbm.RoleBinding
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND subject_type = ? AND subject_id = ?", tenantID, st, sid).
		Find(&out).Error
	return out, err
}

// （用于授权器预热）拉取某成员“有效”的所有绑定：直绑 + 通过 Assignment 拉来的绑定
func (r *RoleBindingRepository) ListEffectiveForMember(ctx context.Context, tenantID, memberID uint64) ([]dbm.RoleBinding, error) {
	tRB := (&dbm.RoleBinding{}).GetTableName(true)
	tMA := (&dbm.MemberAssignment{}).GetTableName(true)

	var out []dbm.RoleBinding
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT rb.* FROM `+tRB+` rb
		WHERE rb.tenant_id = ? AND rb.subject_type = ? AND rb.subject_id = ?
		UNION
		SELECT DISTINCT rb.* FROM `+tRB+` rb
		JOIN `+tMA+` ma
		  ON ma.tenant_id = rb.tenant_id
		 AND rb.subject_type = ma.dim_type
		 AND rb.subject_id = ma.dim_id
		WHERE rb.tenant_id = ? AND ma.member_id = ?`,
		tenantID, dbm.SubMember, memberID,
		tenantID, memberID,
	).Scan(&out).Error
	return out, err
}

// —— 修正 1：按“成员”列出其角色（推荐用此函数），不再使用 userID ——

// ListRolesByMember：列出某成员在租户下的所有“直绑”角色（不含间接）
func (r *RoleBindingRepository) ListRolesByMember(ctx context.Context, tenantID, memberID uint64) ([]dbm.Role, error) {
	var roles []dbm.Role
	err := r.db.WithContext(ctx).
		Table((&dbm.Role{}).GetTableName(true)+" AS r").
		Select("r.*").
		Joins("JOIN "+(&dbm.RoleBinding{}).GetTableName(true)+" AS rb ON rb.role_id = r.id").
		Where("rb.tenant_id = ? AND rb.subject_type = ? AND rb.subject_id = ?", tenantID, dbm.SubMember, memberID).
		Find(&roles).Error
	return roles, err
}

// —— 修正 2：若你确实需要以 userID 作为入参（兼容旧调用），先映射到 memberID ——

// ListMemberRoles：保持函数名，但含义改为“按 userID 查直绑角色（内部先找 memberID）”
func (r *RoleBindingRepository) ListMemberRoles(ctx context.Context, tenantID, userID uint64) ([]dbm.Role, error) {
	// 1) userID -> memberID（一个 user 在一个租户只有一个 member）
	var memberID uint64
	if err := r.db.WithContext(ctx).
		Table((&dbm.Member{}).GetTableName(true)).
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Pluck("id", &memberID).Error; err != nil {
		return nil, err
	}
	if memberID == 0 {
		return []dbm.Role{}, nil
	}
	// 2) 复用按“成员”查角色
	return r.ListRolesByMember(ctx, tenantID, memberID)
}

// —— 修正 3：按 code 绑定角色给成员（走 role_binding） ——

// AssignRolesByCodes：把若干 code 的角色直绑给成员（幂等）
func (r *RoleBindingRepository) AssignRolesByCodes(ctx context.Context, tenantID, memberID uint64, codes ...string) error {
	if tenantID == 0 || memberID == 0 || len(codes) == 0 {
		return nil
	}
	// 只匹配本租户的 tenant-scope 角色
	var roles []dbm.Role
	if err := r.db.WithContext(ctx).
		Where("scope = ? AND tenant_id = ? AND code IN ?", "tenant", tenantID, codes).
		Find(&roles).Error; err != nil {
		return err
	}
	// 逐个 upsert 绑定
	for _, role := range roles {
		rb := &dbm.RoleBinding{
			TenantID:    tenantID,
			RoleID:      role.ID,
			SubjectType: dbm.SubMember,
			SubjectID:   memberID,
			// DataScope/ScopeDim/ScopeID/Condition 可按需要补
		}
		if err := r.db.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(rb).Error; err != nil {
			return err
		}
	}
	return nil
}
