// internal/service/iam/rbac_service.go
package iam

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/internal/service"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"strings"

	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	rb "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

type RBACService struct {
	db  *gorm.DB
	rpr *rb.RolePermissionRepository
	rbr *rb.RoleBindingRepository
	pr  *rb.PermissionRepository
	rr  *rb.RoleRepository
}

func NewRBACService(db *gorm.DB) *RBACService {
	return &RBACService{
		db:  db,
		rpr: rb.NewRolePermissionRepository(db), // 角色⇄权限（授予/查） :contentReference[oaicite:6]{index=6}
		rbr: rb.NewRoleBindingRepository(db),    // 角色⇄主体（绑定/解绑定/聚合） :contentReference[oaicite:7]{index=7}
		pr:  rb.NewPermissionRepository(db),     // 权限Upsert/鉴权EXISTS版 :contentReference[oaicite:8]{index=8}
		rr:  rb.NewRoleRepository(db),
	}
}

// ========== 1) 角色 ⇄ 权限 ==========
type PermTriple struct{ Plugin, Resource, Action string }

// A. 通过权限ID授予（最快路径）
func (s *RBACService) GrantPermsByIDs(ctx context.Context, roleID uint64, permIDs []uint64) error {
	if len(permIDs) == 0 {
		return nil
	}
	cur, err := s.rr.GetFirst(ctx, map[string]any{"id": roleID})
	if err != nil || cur == nil {
		return gorm.ErrRecordNotFound
	}
	// 非 root：仅允许给本租户的 tenant 角色授权
	if !service.IsRoot(ctx) {
		if strings.ToLower(cur.Scope) != "tenant" || cur.TenantID == 0 || cur.TenantID != auth.GetTenantID(ctx) {
			return errors.New("forbidden")
		}
	}
	return s.rpr.BindPermissions(ctx, roleID, permIDs...) // 幂等 upsert :contentReference[oaicite:9]{index=9}
}

// B. 通过 (plugin,resource,action) 授予（便于API）
func (s *RBACService) GrantPermsByTriples(ctx context.Context, roleID uint64, triples []PermTriple) error {
	if len(triples) == 0 {
		return nil
	}
	// 1) 先 upsert 权限行（幂等）
	toUpsert := make([]dbm.Permission, 0, len(triples))
	for _, t := range triples {
		toUpsert = append(toUpsert, dbm.Permission{Plugin: t.Plugin, Resource: t.Resource, Action: t.Action})
	}
	if err := s.pr.UpsertBatch(ctx, toUpsert); err != nil { // :contentReference[oaicite:10]{index=10}
		return err
	}
	// 2) 查回这些权限ID
	var permIDs []uint64
	if err := s.db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("(plugin,resource,action) IN ?", triplesToTuples(triples)).
		Pluck("id", &permIDs).Error; err != nil {
		return err
	}
	return s.GrantPermsByIDs(ctx, roleID, permIDs)
}

func (s *RBACService) ListPermsOfRole(ctx context.Context, roleID uint64) ([]dbm.Permission, error) {
	return s.rpr.ListRolePermissions(ctx, roleID) // :contentReference[oaicite:11]{index=11}
}

// ========== 2) 角色 ⇄ 主体（成员/部门/…） ==========
func (s *RBACService) BindRoleToMember(ctx context.Context, tenantID, roleID, memberID uint64) error {
	cur, err := s.rr.GetFirst(ctx, map[string]any{"id": roleID})
	if err != nil || cur == nil {
		return gorm.ErrRecordNotFound
	}
	if strings.ToLower(cur.Scope) == string(iam.RoleScopeSystem) {
		return errors.New("cannot bind system-scope role")
	}

	if !service.IsRoot(ctx) {
		if tenantID == 0 || tenantID != auth.GetTenantID(ctx) || cur.TenantID != tenantID {
			return errors.New("forbidden")
		}
	}
	// subject_type 用你模型里的常量：SubMember
	return s.rbr.Create(ctx, &dbm.RoleBinding{
		TenantID: tenantID, RoleID: roleID, SubjectType: dbm.SubMember, SubjectID: memberID,
	}) // 幂等 on-conflict :contentReference[oaicite:12]{index=12}
}

func (s *RBACService) UnbindRoleFromMember(ctx context.Context, tenantID, roleBindingID uint64) error {
	// 你的 repo 的 Delete 是按 (tenant_id, id) 删除
	if !service.IsRoot(ctx) && tenantID != auth.GetTenantID(ctx) {
		return errors.New("forbidden")
	}
	return s.rbr.Delete(ctx, tenantID, roleBindingID) // :contentReference[oaicite:13]{index=13}
}

// ========== 3) 鉴权（root 放行；直绑 + 维度间接绑定） ==========
func (s *RBACService) Enforce(ctx context.Context, tenantID, memberID uint64, plugin, resource, action string) (bool, error) {
	if service.IsRoot(ctx) {
		return true, nil
	}
	if tenantID == 0 {
		tenantID = auth.GetTenantID(ctx)
	}
	if tenantID == 0 || memberID == 0 {
		return false, errors.New("tenant/member required")
	}
	// 复用 EXISTS 版绑定鉴权（直绑 + Assignment 聚合）
	return s.pr.MemberHasPermissionViaBinding(ctx, tenantID, memberID, resource, action) // :contentReference[oaicite:14]{index=14}
}

func triplesToTuples(ts []PermTriple) [][3]string {
	out := make([][3]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, [3]string{t.Plugin, t.Resource, t.Action})
	}
	return out
}

// RevokePermissionsFromRole：按权限ID列表撤销角色的权限
func (s *RBACService) RevokePermissionsFromRole(ctx context.Context, roleID uint64, permIDs []uint64) error {
	if len(permIDs) == 0 {
		return nil
	}

	// 1) 先拿角色并做作用域/租户校验（与 GrantPermsByIDs 保持一致）
	cur, err := s.rr.GetFirst(ctx, map[string]any{"id": roleID})
	if err != nil || cur == nil {
		return gorm.ErrRecordNotFound
	}
	if !service.IsRoot(ctx) {
		if strings.ToLower(cur.Scope) != "tenant" || cur.TenantID == 0 || cur.TenantID != auth.GetTenantID(ctx) {
			return errors.New("forbidden")
		}
	}

	// 2) 删除绑定（按 role_id + permission_id）
	return s.db.WithContext(ctx).
		Table("iam_role_permission").
		Where("role_id = ? AND permission_id IN ?", roleID, permIDs).
		Delete(nil).Error
}
