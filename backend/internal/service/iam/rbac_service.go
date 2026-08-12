// internal/service/iam/rbac_service.go
package iam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/google/uuid"

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
type PermTriple struct{ Module, Resource, Action string }

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
	if err := s.rpr.BindPermissions(ctx, roleID, permIDs...); err != nil {
		return err
	}
	return invalidateAgentEffectivePermissionsIAMCache(ctx, cur.TenantUUID)
}

// B. 通过 (module,resource,action) 授予（便于API）
func (s *RBACService) GrantPermsByTriples(ctx context.Context, actor ActorContext, roleID uint64, triples []PermTriple) error {
	if len(triples) == 0 {
		return nil
	}
	// 1) 先 upsert 权限行（幂等）
	toUpsert := make([]dbm.Permission, 0, len(triples))
	for _, t := range triples {
		toUpsert = append(toUpsert, dbm.Permission{Module: t.Module, Resource: t.Resource, Action: t.Action})
	}
	if err := s.pr.UpsertBatch(ctx, toUpsert); err != nil { // :contentReference[oaicite:10]{index=10}
		return err
	}
	// 2) 查回这些权限ID
	var permIDs []uint64
	if err := s.db.WithContext(ctx).
		Model(&dbm.Permission{}).
		Where("(module,resource,action) IN ?", triplesToTuples(triples)).
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

	var role dbm.Role
	if err := s.db.WithContext(ctx).
		Where("tenant_uuid = ? AND id = ?", tenantUUID, roleID).
		First(&role).Error; err != nil {
		return err
	}
	var member dbm.Member
	if err := s.db.WithContext(ctx).
		Where("tenant_uuid = ? AND id = ?", tenantUUID, memberID).
		First(&member).Error; err != nil {
		return err
	}
	if role.UUID == uuid.Nil || member.UUID == uuid.Nil {
		return errors.New("role/member uuid required")
	}

	if err := s.rbr.Create(ctx, &dbm.RoleBinding{
		TenantUUID:  tenantUUID,
		RoleUUID:    role.UUID.String(),
		RoleID:      roleID,
		SubjectType: dbm.SubMember,
		SubjectUUID: member.UUID.String(),
		SubjectID:   memberID,
	}); err != nil {
		return err
	}
	return invalidateAgentEffectivePermissionsIAMCache(ctx, tenantUUID)
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
	if err := s.rbr.Delete(ctx, tenantUUID, roleBindingID); err != nil {
		return err
	}
	return invalidateAgentEffectivePermissionsIAMCache(ctx, tenantUUID)
}

// ========== 3) 鉴权（root 放行；直绑 + 维度间接绑定） ==========
func (s *RBACService) Enforce(ctx context.Context, actor ActorContext, tenantUUID string, memberID uint64, module, resource, action string) (bool, error) {
	if actor.IsRoot {
		return true, nil
	}
	if strings.TrimSpace(tenantUUID) == "" {
		tenantUUID = strings.TrimSpace(actor.TenantUUID)
	}
	if strings.TrimSpace(tenantUUID) == "" || memberID == 0 {
		return false, errors.New("tenant/member required")
	}
	// 复用 EXISTS 版绑定鉴权（直绑 + Assignment 聚合）。
	// module 为空时保持历史兼容，仅按 resource/action 鉴权。
	module = strings.TrimSpace(module)
	if module == "" {
		return s.pr.MemberHasPermissionViaBinding(ctx, tenantUUID, memberID, resource, action)
	}
	return s.pr.MemberHasPermissionViaBindingWithModule(ctx, tenantUUID, memberID, module, resource, action)
}

func triplesToTuples(ts []PermTriple) [][3]string {
	out := make([][3]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, [3]string{t.Module, t.Resource, t.Action})
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
	if err := s.db.WithContext(ctx).
		Table("iam_role_permission").
		Where("role_id = ? AND permission_id IN ?", roleID, permIDs).
		Delete(nil).Error; err != nil {
		return err
	}
	return invalidateAgentEffectivePermissionsIAMCache(ctx, cur.TenantUUID)
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
		expanded, err := s.expandPluginMenuPagePermissionIDs(ctx, ps)
		if err != nil {
			return SetIDsResult{}, err
		}
		for _, id := range expanded {
			validSet[id] = struct{}{}
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
	if len(toAdd) > 0 || len(toRemove) > 0 {
		if err := invalidateAgentEffectivePermissionsIAMCache(ctx, role.TenantUUID); err != nil {
			return SetIDsResult{}, err
		}
	}

	slices.Sort(toAdd)
	slices.Sort(toRemove)
	slices.Sort(skipped)
	return SetIDsResult{Added: toAdd, Removed: toRemove, Now: now, SkippedDeprecated: skipped}, nil
}

type pluginRBACPermissionMeta struct {
	Type                string                      `json:"type"`
	Origin              string                      `json:"origin"`
	PluginID            string                      `json:"plugin_id"`
	Path                string                      `json:"path"`
	Route               string                      `json:"route"`
	Permission          string                      `json:"permission"`
	PagePermissionCodes []string                    `json:"page_permission_codes"`
	ProtocolBindings    []pluginRBACProtocolBinding `json:"protocol_bindings"`
}

type pluginRBACProtocolBinding struct {
	Channel string `json:"channel"`
	Method  string `json:"method"`
	Path    string `json:"path"`
}

func (s *RBACService) expandPluginMenuPagePermissionIDs(ctx context.Context, selected []*dbm.Permission) ([]uint64, error) {
	if s == nil || s.db == nil || len(selected) == 0 {
		return nil, nil
	}
	type sourceMenuPaths struct {
		source              string
		paths               map[string]struct{}
		pagePermissionCodes map[string]struct{}
	}
	menusBySource := map[string]sourceMenuPaths{}
	for _, perm := range selected {
		if perm == nil || strings.TrimSpace(perm.Source) == "" || !strings.HasPrefix(strings.TrimSpace(perm.Source), "plugin:") {
			continue
		}
		meta := decodePluginRBACPermissionMeta(perm.Meta)
		if !strings.EqualFold(strings.TrimSpace(meta.Type), "menu") || !strings.EqualFold(strings.TrimSpace(meta.Origin), "plugin") {
			continue
		}
		paths := pluginMenuAdminPathCandidates(meta.Path, meta.Route)
		pagePermissionCodes := normalizePermissionCodeSet(meta.PagePermissionCodes)
		if len(paths) == 0 && len(pagePermissionCodes) == 0 {
			continue
		}
		source := strings.TrimSpace(perm.Source)
		bucket := menusBySource[source]
		if bucket.paths == nil {
			bucket = sourceMenuPaths{source: source, paths: map[string]struct{}{}, pagePermissionCodes: map[string]struct{}{}}
		}
		for _, item := range paths {
			bucket.paths[item] = struct{}{}
		}
		for item := range pagePermissionCodes {
			bucket.pagePermissionCodes[item] = struct{}{}
		}
		menusBySource[source] = bucket
	}
	if len(menusBySource) == 0 {
		return nil, nil
	}

	expanded := map[uint64]struct{}{}
	for source, bucket := range menusBySource {
		var rows []dbm.Permission
		if err := s.db.WithContext(ctx).
			Where("source = ? AND status = ?", source, dbm.PermissionStatusActive).
			Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			meta := decodePluginRBACPermissionMeta(rows[i].Meta)
			if !strings.EqualFold(strings.TrimSpace(meta.Type), "page") {
				continue
			}
			if permissionCodeInSet(meta.Permission, bucket.pagePermissionCodes) || pluginPagePermissionMatchesMenuPath(meta, bucket.paths) {
				expanded[rows[i].ID] = struct{}{}
			}
		}
	}
	if len(expanded) == 0 {
		return nil, nil
	}
	ids := make([]uint64, 0, len(expanded))
	for id := range expanded {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}

func normalizePermissionCodeSet(items []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, raw := range items {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		out[item] = struct{}{}
	}
	return out
}

func permissionCodeInSet(permission string, set map[string]struct{}) bool {
	permission = strings.TrimSpace(permission)
	if permission == "" || len(set) == 0 {
		return false
	}
	_, ok := set[permission]
	return ok
}

func decodePluginRBACPermissionMeta(raw []byte) pluginRBACPermissionMeta {
	var meta pluginRBACPermissionMeta
	if len(raw) == 0 {
		return meta
	}
	_ = json.Unmarshal(raw, &meta)
	return meta
}

func pluginMenuAdminPathCandidates(values ...string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		candidates := []string{value}
		if !strings.HasPrefix(value, "/") {
			candidates = append(candidates, "/"+value)
		}
		for _, candidate := range candidates {
			normalized := normalizePluginRBACPath(candidate)
			if normalized == "" {
				continue
			}
			seen[normalized] = struct{}{}
			if !strings.HasPrefix(normalized, "/admin/") && normalized != "/admin" {
				seen[normalizePluginRBACPath("/admin"+normalized)] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for item := range seen {
		out = append(out, item)
	}
	slices.Sort(out)
	return out
}

func pluginPagePermissionMatchesMenuPath(meta pluginRBACPermissionMeta, menuPaths map[string]struct{}) bool {
	for _, binding := range meta.ProtocolBindings {
		if !strings.EqualFold(strings.TrimSpace(binding.Channel), "rest") {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(binding.Method), "GET") {
			continue
		}
		if _, ok := menuPaths[normalizePluginRBACPath(binding.Path)]; ok {
			return true
		}
	}
	return false
}

func normalizePluginRBACPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	for strings.Contains(value, "//") {
		value = strings.ReplaceAll(value, "//", "/")
	}
	if len(value) > 1 {
		value = strings.TrimRight(value, "/")
	}
	return value
}

func invalidateAgentEffectivePermissionsIAMCache(ctx context.Context, tenantUUID string) error {
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return nil
	}
	store := cache.GetCache()
	if store == nil {
		return nil
	}
	_, err := store.Increment(ctx, fmt.Sprintf("agentauthz:effective:iam-version:%s", strings.ToLower(tenantUUID)), 1)
	return err
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
