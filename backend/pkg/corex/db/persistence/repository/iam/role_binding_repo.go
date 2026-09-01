// pkg/corex/db/persistence/repository/iam/role_binding_repo.go
package iam

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
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

// 幂等创建绑定（建议你在表上建唯一键：tenant_uuid,role_id,subject_type,subject_id）
func (r *RoleBindingRepository) Create(ctx context.Context, rb *dbm.RoleBinding) error {
	if rb == nil ||
		strings.TrimSpace(rb.TenantUUID) == "" ||
		isBlankBusinessUUID(rb.RoleUUID) ||
		rb.RoleID == 0 ||
		isBlankBusinessUUID(rb.SubjectUUID) ||
		rb.SubjectID == 0 ||
		rb.SubjectType == "" {
		return errors.New("invalid role binding")
	}
	dataScope := rb.DataScope
	if dataScope == "" {
		dataScope = dbm.ScopeTenant
		rb.DataScope = dataScope
	}
	var existingID uint64
	if err := r.db.WithContext(ctx).
		Model(&dbm.RoleBinding{}).
		Where("tenant_uuid = ? AND subject_type = ? AND subject_uuid = ? AND role_uuid = ? AND data_scope = ?",
			rb.TenantUUID, rb.SubjectType, rb.SubjectUUID, rb.RoleUUID, dataScope).
		Limit(1).
		Pluck("id", &existingID).Error; err != nil {
		return err
	}
	if existingID != 0 {
		rb.ID = existingID
		return nil
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(rb).Error
}

func isBlankBusinessUUID(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || value == uuid.Nil.String()
}

func (r *RoleBindingRepository) Delete(ctx context.Context, tenantUUID string, id uint64) error {
	return r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND id = ?", tenantUUID, id).
		Delete(&dbm.RoleBinding{}).Error
}

func (r *RoleBindingRepository) ListBySubject(ctx context.Context, tenantUUID string, st dbm.SubjectType, sid uint64) ([]dbm.RoleBinding, error) {
	var out []dbm.RoleBinding
	err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND subject_type = ? AND subject_id = ?", tenantUUID, st, sid).
		Find(&out).Error
	return out, err
}

// （用于授权器预热）拉取某成员“有效”的所有绑定：直绑 + 通过 Assignment 拉来的绑定
func (r *RoleBindingRepository) ListEffectiveForMember(ctx context.Context, tenantUUID string, memberID uint64) ([]dbm.RoleBinding, error) {
	tRB := (&dbm.RoleBinding{}).GetTableName(true)
	tMA := (&dbm.MemberAssignment{}).GetTableName(true)

	var out []dbm.RoleBinding
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT rb.* FROM `+tRB+` rb
		WHERE rb.tenant_uuid = ? AND rb.subject_type = ? AND rb.subject_id = ?
		UNION
		SELECT DISTINCT rb.* FROM `+tRB+` rb
		JOIN `+tMA+` ma
		  ON ma.tenant_uuid = rb.tenant_uuid
		 AND rb.subject_id = ma.dim_id
		 AND rb.subject_type = CASE ma.dim_type
		   WHEN 'ORG' THEN 'ORG_UNIT'
		   WHEN 'TEAM' THEN 'TEAM'
		   WHEN 'POSITION' THEN 'POSITION'
		   WHEN 'GROUP' THEN 'GROUP'
		 END
		WHERE rb.tenant_uuid = ? AND ma.member_id = ?`,
		tenantUUID, dbm.SubMember, memberID,
		tenantUUID, memberID,
	).Scan(&out).Error
	return out, err
}

// —— 修正 1：按“成员”列出其角色（推荐用此函数），不再使用 userID ——

// ListRolesByMember：列出某成员在租户下的所有“直绑”角色（不含间接）
func (r *RoleBindingRepository) ListRolesByMember(ctx context.Context, tenantUUID string, memberID uint64) ([]dbm.Role, error) {
	var roles []dbm.Role
	err := r.db.WithContext(ctx).
		Table((&dbm.Role{}).GetTableName(true)+" AS r").
		Select("r.*").
		Joins("JOIN "+(&dbm.RoleBinding{}).GetTableName(true)+" AS rb ON rb.role_uuid = CAST(r.uuid AS TEXT)").
		Where("rb.tenant_uuid = ? AND rb.subject_type = ? AND rb.subject_id = ?", tenantUUID, dbm.SubMember, memberID).
		Find(&roles).Error
	return roles, err
}

// —— 修正 2：若你确实需要以 userID 作为入参（兼容旧调用），先映射到 memberID ——

// ListMemberRoles：保持函数名，但含义改为“按 userID 查直绑角色（内部先找 memberID）”
func (r *RoleBindingRepository) ListMemberRoles(ctx context.Context, tenantUUID string, userID uint64) ([]dbm.Role, error) {
	// 1) userID -> memberID（一个 user 在一个租户只有一个 member）
	var memberID uint64
	if err := r.db.WithContext(ctx).
		Table((&dbm.Member{}).GetTableName(true)).
		Where("tenant_uuid = ? AND user_id = ?", tenantUUID, userID).
		Pluck("id", &memberID).Error; err != nil {
		return nil, err
	}
	if memberID == 0 {
		return []dbm.Role{}, nil
	}
	// 2) 复用按“成员”查角色
	return r.ListRolesByMember(ctx, tenantUUID, memberID)
}

// —— 修正 3：按 code 绑定角色给成员（走 role_binding） ——

// AssignRolesByCodes：把若干 code 的角色直绑给成员（幂等）
func (r *RoleBindingRepository) AssignRolesByCodes(ctx context.Context, tenantUUID string, memberID uint64, codes ...string) error {
	if strings.TrimSpace(tenantUUID) == "" || memberID == 0 || len(codes) == 0 {
		return nil
	}
	// 只匹配本租户的 tenant-scope 角色
	var roles []dbm.Role
	if err := r.db.WithContext(ctx).
		Where("scope = ? AND tenant_uuid = ? AND code IN ?", "tenant", tenantUUID, codes).
		Find(&roles).Error; err != nil {
		return err
	}
	var member dbm.Member
	if err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND id = ?", tenantUUID, memberID).
		First(&member).Error; err != nil {
		return err
	}
	if member.UUID == uuid.Nil {
		return errors.New("member missing uuid")
	}
	memberUUID := member.UUID.String()
	// 逐个 upsert 绑定
	for _, role := range roles {
		if role.UUID == uuid.Nil {
			return errors.New("role missing uuid")
		}
		roleUUID := role.UUID.String()
		rb := &dbm.RoleBinding{
			TenantUUID:  tenantUUID,
			RoleUUID:    roleUUID,
			RoleID:      role.ID,
			SubjectType: dbm.SubMember,
			SubjectUUID: memberUUID,
			SubjectID:   memberID,
			// DataScope/ScopeDim/ScopeID/Condition 可按需要补
		}
		if err := r.Create(ctx, rb); err != nil {
			return err
		}
	}
	return nil
}
