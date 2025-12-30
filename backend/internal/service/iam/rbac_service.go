// internal/service/iam/rbac_service.go
package iam

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/utils"

	"gorm.io/gorm"

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
func (s *RBACService) GrantPermsByIDs(ctx context.Context, actor ActorContext, roleID uint64, permIDs []uint64) error {
	if len(permIDs) == 0 {
		return nil
	}
	cur, err := s.rr.GetFirst(ctx, map[string]any{"id": roleID})
	if err != nil || cur == nil {
		return gorm.ErrRecordNotFound
	}
	// 非 root：仅允许给本租户的 tenant 角色授权
	if !actor.IsRoot {
		ctxTenant := strings.TrimSpace(actor.TenantUUID)
		if ctxTenant == "" {
			return errors.New("tenant context required")
		}
		if strings.ToLower(cur.Scope) != "tenant" || strings.TrimSpace(cur.TenantUUID) == "" || strings.TrimSpace(cur.TenantUUID) != ctxTenant {
			return errors.New("forbidden")
		}
	}
	return s.rpr.BindPermissions(ctx, roleID, permIDs...) // 幂等 upsert :contentReference[oaicite:9]{index=9}
}

// B. 通过 (plugin,resource,action) 授予（便于API）
func (s *RBACService) GrantPermsByTriples(ctx context.Context, actor ActorContext, roleID uint64, triples []PermTriple) error {
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
	return s.GrantPermsByIDs(ctx, actor, roleID, permIDs)
}

func (s *RBACService) ListPermsOfRole(ctx context.Context, roleID uint64) ([]dbm.Permission, error) {
	return s.rpr.ListRolePermissions(ctx, roleID) // :contentReference[oaicite:11]{index=11}
}

// ========== 2) 角色 ⇄ 主体（成员/部门/…） ==========
func (s *RBACService) BindRoleToMember(ctx context.Context, actor ActorContext, tenantUUID string, roleID, memberID uint64) error {
	cur, err := s.rr.GetFirst(ctx, map[string]any{"id": roleID})
	if err != nil || cur == nil {
		return gorm.ErrRecordNotFound
	}
	if strings.ToLower(cur.Scope) == string(iam.RoleScopeSystem) {
		return errors.New("cannot bind system-scope role")
	}

	if !actor.IsRoot {
		ctxTenant := strings.TrimSpace(actor.TenantUUID)
		if ctxTenant == "" {
			return errors.New("tenant context required")
		}
		tenantUUID = strings.TrimSpace(tenantUUID)
		if tenantUUID == "" {
			tenantUUID = ctxTenant
		}
		if tenantUUID == "" || tenantUUID != ctxTenant || strings.TrimSpace(cur.TenantUUID) != ctxTenant {
			return errors.New("forbidden")
		}
	} else {
		tenantUUID = strings.TrimSpace(tenantUUID)
	}

	if tenantUUID == "" {
		return errors.New("tenant uuid required")
	}

	// subject_type 用你模型里的常量：SubMember
	return s.rbr.Create(ctx, &dbm.RoleBinding{
		TenantUUID: tenantUUID, RoleID: roleID, SubjectType: dbm.SubMember, SubjectID: memberID,
	}) // 幂等 on-conflict :contentReference[oaicite:12]{index=12}
}

func (s *RBACService) UnbindRoleFromMember(ctx context.Context, actor ActorContext, tenantUUID string, roleBindingID uint64) error {
	if !actor.IsRoot {
		ctxTenant := strings.TrimSpace(actor.TenantUUID)
		if ctxTenant == "" {
			return errors.New("tenant context required")
		}
		tenantUUID = strings.TrimSpace(tenantUUID)
		if tenantUUID == "" {
			tenantUUID = ctxTenant
		}
		if tenantUUID != ctxTenant {
			return errors.New("forbidden")
		}
	}
	if strings.TrimSpace(tenantUUID) == "" {
		return errors.New("tenant uuid required")
	}
	return s.rbr.Delete(ctx, tenantUUID, roleBindingID) // :contentReference[oaicite:13]{index=13}
}

// ========== 3) 鉴权（root 放行；直绑 + 维度间接绑定） ==========
func (s *RBACService) Enforce(ctx context.Context, actor ActorContext, tenantUUID string, memberID uint64, plugin, resource, action string) (bool, error) {
	if actor.IsRoot {
		return true, nil
	}
	if strings.TrimSpace(tenantUUID) == "" {
		tenantUUID = strings.TrimSpace(actor.TenantUUID)
	}
	if strings.TrimSpace(tenantUUID) == "" || memberID == 0 {
		return false, errors.New("tenant/member required")
	}
	// 复用 EXISTS 版绑定鉴权（直绑 + Assignment 聚合）
	return s.pr.MemberHasPermissionViaBinding(ctx, tenantUUID, memberID, resource, action) // :contentReference[oaicite:14]{index=14}
}

func triplesToTuples(ts []PermTriple) [][3]string {
	out := make([][3]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, [3]string{t.Plugin, t.Resource, t.Action})
	}
	return out
}

// RevokePermissionsFromRole：按权限ID列表撤销角色的权限
func (s *RBACService) RevokePermissionsFromRole(ctx context.Context, actor ActorContext, roleID uint64, permIDs []uint64) error {
	if len(permIDs) == 0 {
		return nil
	}

	// 1) 先拿角色并做作用域/租户校验（与 GrantPermsByIDs 保持一致）
	cur, err := s.rr.GetFirst(ctx, map[string]any{"id": roleID})
	if err != nil || cur == nil {
		return gorm.ErrRecordNotFound
	}
	if !actor.IsRoot {
		ctxTenant := strings.TrimSpace(actor.TenantUUID)
		if ctxTenant == "" {
			return errors.New("tenant context required")
		}
		if strings.ToLower(cur.Scope) != "tenant" || strings.TrimSpace(cur.TenantUUID) == "" || strings.TrimSpace(cur.TenantUUID) != ctxTenant {
			return errors.New("forbidden")
		}
	}

	// 2) 删除绑定（按 role_id + permission_id）
	return s.db.WithContext(ctx).
		Table("iam_role_permission").
		Where("role_id = ? AND permission_id IN ?", roleID, permIDs).
		Delete(nil).Error
}

func (s *RBACService) SetPermissionIDs(ctx context.Context, actor ActorContext, roleID uint64, wantIDs []uint64) (SetIDsResult, error) {
	// 角色查询：你的 GetFirst 可能返回 (*T,nil)，要判空
	role, err := s.rr.GetFirst(ctx, "id = ?", roleID)
	if err != nil {
		return SetIDsResult{}, err
	}
	if role == nil { // ✅ 防 NPE
		return SetIDsResult{}, service.ErrRoleNotFound
	}

	// 鉴权：非 root 只能改本租户
	if !actor.IsRoot && strings.TrimSpace(role.TenantUUID) != strings.TrimSpace(actor.TenantUUID) {
		return SetIDsResult{}, service.ErrForbidden
	}
	// 如果你模型里有 IsSystem 并且想限制，可加：
	// if !ac.IsRoot && role.IsSystem { return SetIDsResult{}, ErrForbidden }

	// 去重
	wantIDs = utils.UniqUint64(wantIDs)

	// 当前已有
	curIDs, err := s.rpr.ListPermissionIDsOfRole(ctx, roleID)
	if err != nil {
		return SetIDsResult{}, err
	}
	curSet := make(map[uint64]struct{}, len(curIDs))
	for _, id := range curIDs {
		curSet[id] = struct{}{}
	}

	// 只允许 active 的权限；跳过 deprecated
	validSet := make(map[uint64]struct{})
	var skipped []uint64
	if len(wantIDs) > 0 {
		ps, err := s.pr.FindByIDs(ctx, wantIDs) // ✅ 仓储方法，返回 []*dbm.Permission
		if err != nil {
			return SetIDsResult{}, err
		}
		for _, p := range ps {
			st := strings.ToLower(strings.TrimSpace(string(p.Status))) // ✅ PermissionStatus → string
			if st == "" || st == strings.ToLower(string(dbm.PermissionStatusActive)) {
				validSet[p.ID] = struct{}{}
			} else {
				skipped = append(skipped, p.ID)
			}
		}
	}

	var toAdd, toRemove []uint64
	for id := range validSet {
		if _, ok := curSet[id]; !ok {
			toAdd = append(toAdd, id)
		}
	}
	for id := range curSet {
		if _, ok := validSet[id]; !ok {
			toRemove = append(toRemove, id)
		}
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(toAdd) > 0 {
			if err := s.rpr.GrantByIDsTx(tx, roleID, toAdd); err != nil {
				return err
			}
		}
		if len(toRemove) > 0 {
			if err := s.rpr.RevokeByIDsTx(tx, roleID, toRemove); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return SetIDsResult{}, err
	}

	now, err := s.rpr.ListPermissionIDsOfRole(ctx, roleID)
	if err != nil {
		return SetIDsResult{}, err
	}

	slices.Sort(toAdd)
	slices.Sort(toRemove)
	slices.Sort(skipped)
	return SetIDsResult{Added: toAdd, Removed: toRemove, Now: now, SkippedDeprecated: skipped}, nil
}

func (s *RBACService) ListPermissionIDs(ctx context.Context, actor ActorContext, roleID uint64) ([]uint64, error) {
	role, err := s.rr.GetFirst(ctx, "id = ?", roleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, service.ErrRoleNotFound
	}

	if !actor.IsRoot && strings.TrimSpace(role.TenantUUID) != strings.TrimSpace(actor.TenantUUID) {
		return nil, service.ErrForbidden
	}
	return s.rpr.ListPermissionIDsOfRole(ctx, roleID)
}
