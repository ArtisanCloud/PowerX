package apikeypermissions

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	apikeycache "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeycache"
	modelsiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	igwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var exactPluginID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

type APIKeyOwnerService struct{ db *gorm.DB }

func NewAPIKeyOwnerService(db *gorm.DB) *APIKeyOwnerService { return &APIKeyOwnerService{db: db} }

type APIKeyOwnerResult struct {
	KeyID              string                   `json:"key_id"`
	TenantUUID         string                   `json:"tenant_uuid"`
	PluginIDs          []string                 `json:"plugin_ids"`
	BindingMode        string                   `json:"binding_mode"`
	PermissionBindings []OwnerPermissionBinding `json:"permission_bindings,omitempty"`
	UpdatedAt          string                   `json:"updated_at"`
}

type OwnerPermissionBinding struct {
	Scope           string `json:"scope"`
	Action          string `json:"action"`
	ResourceType    string `json:"resource_type"`
	ResourcePattern string `json:"resource_pattern"`
	PluginID        string `json:"plugin_id"`
	Effect          string `json:"effect"`
}

type OwnerPolicy struct {
	Version   int                      `json:"version"`
	Mode      string                   `json:"mode"`
	PluginIDs []string                 `json:"plugin_ids,omitempty"`
	Bindings  []OwnerPermissionBinding `json:"bindings,omitempty"`
}

func (p OwnerPolicy) Encode() datatypes.JSON { raw, _ := json.Marshal(p); return datatypes.JSON(raw) }
func (p OwnerPolicy) Owners() []string {
	if p.Mode == "key" {
		if p.PluginIDs == nil {
			return []string{}
		}
		return p.PluginIDs
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, binding := range p.Bindings {
		if !seen[binding.PluginID] {
			seen[binding.PluginID] = true
			ids = append(ids, binding.PluginID)
		}
	}
	sort.Strings(ids)
	return ids
}

func ResolveOwnerPolicy(raw datatypes.JSON, grants []models.IntegrationGatewayAPIKeyPermission) (OwnerPolicy, error) {
	policy := OwnerPolicy{Version: 1, Mode: "legacy_permissions"}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &policy); err != nil {
			return policy, ownerError(dto.NewInternal("插件owner绑定数据无效", err), "API_KEY_PLUGIN_OWNER_INVALID")
		}
		if policy.Version != 1 || (policy.Mode != "key" && policy.Mode != "legacy_permissions") {
			return policy, ownerError(dto.NewInternal("插件owner绑定版本或模式无效", nil), "API_KEY_PLUGIN_OWNER_INVALID")
		}
	} else {
		for _, g := range grants {
			if g.Effect == "allow" && exactPluginID.MatchString(g.PluginID) {
				policy.Bindings = append(policy.Bindings, OwnerPermissionBinding{g.Scope, g.Action, g.ResourceType, g.ResourcePattern, g.PluginID, g.Effect})
			}
		}
	}
	ids, err := NormalizePluginOwners(policy.Owners())
	if err != nil {
		return policy, err
	}
	if policy.Mode == "key" {
		policy.PluginIDs = ids
	} else {
		for _, b := range policy.Bindings {
			if !exactPluginID.MatchString(b.PluginID) || b.Scope == "" || b.Action == "" || b.ResourceType == "" || b.ResourcePattern == "" || b.Effect != "allow" {
				return policy, ownerError(dto.NewInternal("旧插件owner权限绑定无效", nil), "API_KEY_PLUGIN_OWNER_INVALID")
			}
		}
	}
	return policy, nil
}

func BindOwnerPolicy(keyID uuid.UUID, base []models.IntegrationGatewayAPIKeyPermission, policy OwnerPolicy) ([]models.IntegrationGatewayAPIKeyPermission, error) {
	if policy.Mode == "key" {
		return BindPluginOwners(keyID, base, policy.PluginIDs)
	}
	out := []models.IntegrationGatewayAPIKeyPermission{}
	for _, item := range base {
		owners := []string{}
		for _, b := range policy.Bindings {
			if b.Scope == item.Scope && b.Action == item.Action && b.ResourceType == item.ResourceType && b.ResourcePattern == item.ResourcePattern && b.Effect == item.Effect && (item.PluginID == "" || item.PluginID == b.PluginID) {
				owners = append(owners, b.PluginID)
			}
		}
		// A new owner-sensitive grant is never inferred from another permission.
		items, err := BindPluginOwners(keyID, []models.IntegrationGatewayAPIKeyPermission{item}, owners)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func ownerError(err *dto.AppError, code string) error {
	err.Code = code
	return dto.WithDetails(err, map[string]any{"reason_code": code})
}

func NormalizePluginOwners(ids []string) ([]string, error) {
	if len(ids) > 32 {
		return nil, ownerError(dto.NewBadRequest("最多授权32个插件owner", nil), "API_KEY_PLUGIN_OWNER_INVALID")
	}
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if !exactPluginID.MatchString(id) {
			return nil, ownerError(dto.NewBadRequest("插件owner必须是完整插件ID，不允许空值、通配符或路径", nil), "API_KEY_PLUGIN_OWNER_INVALID")
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Owner scope is durable independently of the current permission rows.
func EffectivePluginOwners(raw datatypes.JSON, grants []models.IntegrationGatewayAPIKeyPermission) ([]string, error) {
	policy, err := ResolveOwnerPolicy(raw, grants)
	return policy.Owners(), err
}
func EncodePluginOwners(ids []string) datatypes.JSON {
	if ids == nil {
		ids = []string{}
	}
	return (OwnerPolicy{Version: 1, Mode: "key", PluginIDs: ids}).Encode()
}

func BindPluginOwners(keyID uuid.UUID, base []models.IntegrationGatewayAPIKeyPermission, owners []string) ([]models.IntegrationGatewayAPIKeyPermission, error) {
	out := []models.IntegrationGatewayAPIKeyPermission{}
	seen := map[string]bool{}
	appendGrant := func(item models.IntegrationGatewayAPIKeyPermission, owner string) {
		item.ID = 0
		item.UUID = uuid.Nil
		item.APIKeyUUID = keyID
		item.PluginID = owner
		item.CreatedAt = time.Time{}
		item.UpdatedAt = time.Time{}
		key := strings.Join([]string{item.Scope, item.Action, item.ResourceType, item.ResourcePattern, item.PluginID, item.Effect}, "\x00")
		if !seen[key] {
			seen[key] = true
			out = append(out, item)
		}
	}
	for _, item := range base {
		if item.PluginID != "" {
			found := false
			for _, owner := range owners {
				if owner == item.PluginID {
					found = true
					break
				}
			}
			if !found {
				return nil, ownerError(dto.NewBadRequest("Profile权限指定的插件与Key owner绑定不一致", nil), "API_KEY_PLUGIN_OWNER_CONFLICT")
			}
			appendGrant(item, item.PluginID)
		} else if len(owners) == 0 {
			appendGrant(item, "")
		} else {
			for _, owner := range owners {
				appendGrant(item, owner)
			}
		}
	}
	return out, nil
}

// Owner configuration is an Admin user action, never an API Key/STS operation.
func RequireOwnerAdministrator(ctx context.Context, tenant string) error {
	claims := reqctx.GetClaims(ctx)
	if claims == nil || claims.UserID == 0 || claims.Issuer == "powerx-sts" || reqctx.AuthenticatedAPIKeyHash(ctx) != "" {
		return ownerError(dto.NewForbidden("插件owner绑定需要管理员用户身份", nil), "API_KEY_OWNER_ADMIN_REQUIRED")
	}
	current, err := reqctx.RequireTenantUUID(ctx)
	if err != nil || current != tenant {
		return ownerError(dto.NewForbidden("租户上下文不匹配", err), "API_KEY_OWNER_TENANT_MISMATCH")
	}
	if claims.IsRoot {
		return nil
	}
	for _, role := range claims.Roles {
		if role == "role_admin" || role == "system_admin" {
			return nil
		}
	}
	return ownerError(dto.NewForbidden("插件owner绑定需要当前租户管理员权限", nil), "API_KEY_OWNER_ADMIN_REQUIRED")
}

func (s *APIKeyOwnerService) ProfileGrants(ctx context.Context, tx *gorm.DB, profileID uint64) ([]models.IntegrationGatewayAPIKeyPermission, error) {
	ids, err := iamrepo.NewAPIKeyProfilePermissionRepository(tx).ListPermissionIDsOfProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	rows, err := iamrepo.NewPermissionRepository(tx).FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := []models.IntegrationGatewayAPIKeyPermission{}
	for _, p := range rows {
		if p == nil || p.Status != modelsiam.PermissionStatusActive || !p.AllowAPIKey {
			continue
		}
		resolved, ok := ResolvePermission(*p)
		if !ok {
			continue
		}
		out = append(out, models.IntegrationGatewayAPIKeyPermission{Scope: resolved.Scope, Action: resolved.Action, ResourceType: resolved.ResourceType, ResourcePattern: resolved.ResourcePattern, PluginID: resolved.PluginID, Effect: resolved.Effect})
	}
	return out, nil
}

func (s *APIKeyOwnerService) SnapshotKeyTx(ctx context.Context, tx *gorm.DB, tenant string, keyID uuid.UUID) (*models.IntegrationGatewayAPIKey, *OwnerPolicy, error) {
	repo := igwrepo.NewIntegrationGatewayAPIKeyRepository(tx)
	key, err := repo.GetByTenantUUID(ctx, tenant, keyID, false)
	if err != nil {
		return nil, nil, s.mapKeyError(err)
	}
	profile, profileErr := iamrepo.NewAPIKeyProfileRepository(tx).LockTenantProfile(ctx, tenant, key.ProfileID)
	if profileErr != nil {
		return nil, nil, s.mapKeyError(profileErr)
	}
	if profile.Status != 1 {
		return nil, nil, ownerError(dto.NewConflict("API Key Profile已停用", nil), "API_KEY_PROFILE_DISABLED")
	}
	key, err = repo.GetByTenantUUID(ctx, tenant, keyID, true)
	if err != nil {
		return nil, nil, s.mapKeyError(err)
	}
	if key.Status != "active" || (key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now())) {
		return nil, nil, ownerError(dto.NewConflict("API Key已失效，不能更新owner或轮换", nil), "API_KEY_INACTIVE")
	}
	grants, err := igwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(tx).ListByAPIKeyUUID(ctx, keyID)
	if err != nil {
		return nil, nil, err
	}
	policy, err := ResolveOwnerPolicy(key.PluginOwnerPolicy, grants)
	return key, &policy, err
}

func (s *APIKeyOwnerService) SyncProfileTx(ctx context.Context, tx *gorm.DB, tenant string, profileID uint64) (int, int, error) {
	if _, err := iamrepo.NewAPIKeyProfileRepository(tx).LockTenantProfile(ctx, tenant, profileID); err != nil {
		return 0, 0, err
	}
	base, err := s.ProfileGrants(ctx, tx, profileID)
	if err != nil {
		return 0, 0, err
	}
	keys, err := igwrepo.NewIntegrationGatewayAPIKeyRepository(tx).ListActiveByProfile(ctx, tenant, profileID)
	if err != nil {
		return 0, 0, err
	}
	permRepo := igwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(tx)
	for _, key := range keys {
		current, err := permRepo.ListByAPIKeyUUID(ctx, key.UUID)
		if err != nil {
			return 0, 0, err
		}
		policy, err := ResolveOwnerPolicy(key.PluginOwnerPolicy, current)
		if err != nil {
			return 0, 0, err
		}
		items, err := BindOwnerPolicy(key.UUID, base, policy)
		if err != nil {
			return 0, 0, err
		}
		// Freeze legacy exact bindings before replacing rows, including an empty Profile.
		if err := igwrepo.NewIntegrationGatewayAPIKeyRepository(tx).UpdateOwnerPolicy(ctx, tenant, key.UUID, policy.Encode(), key.UpdatedBy); err != nil {
			return 0, 0, err
		}
		if err := permRepo.ReplaceAll(ctx, key.UUID, items); err != nil {
			return 0, 0, err
		}
	}
	return len(keys), len(base), nil
}

func (s *APIKeyOwnerService) GetOwners(ctx context.Context, tenant string, keyID uuid.UUID) (*APIKeyOwnerResult, error) {
	if err := RequireOwnerAdministrator(ctx, tenant); err != nil {
		return nil, err
	}
	key, err := igwrepo.NewIntegrationGatewayAPIKeyRepository(s.db).GetByTenantUUID(ctx, tenant, keyID, false)
	if err != nil {
		return nil, s.mapKeyError(err)
	}
	grants, err := igwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(s.db).ListByAPIKeyUUID(ctx, keyID)
	if err != nil {
		return nil, err
	}
	policy, err := ResolveOwnerPolicy(key.PluginOwnerPolicy, grants)
	if err != nil {
		return nil, err
	}
	return &APIKeyOwnerResult{KeyID: keyID.String(), TenantUUID: tenant, PluginIDs: policy.Owners(), BindingMode: policy.Mode, PermissionBindings: policy.Bindings, UpdatedAt: key.UpdatedAt.UTC().Format(time.RFC3339Nano)}, nil
}

func (s *APIKeyOwnerService) SetOwners(ctx context.Context, tenant string, keyID uuid.UUID, ids []string, actor string) (*APIKeyOwnerResult, error) {
	if err := RequireOwnerAdministrator(ctx, tenant); err != nil {
		return nil, err
	}
	owners, err := NormalizePluginOwners(ids)
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		key, previous, err := s.SnapshotKeyTx(ctx, tx, tenant, keyID)
		if err != nil {
			return err
		}
		base, err := s.ProfileGrants(ctx, tx, key.ProfileID)
		if err != nil {
			return err
		}
		grants, err := BindPluginOwners(keyID, base, owners)
		if err != nil {
			return err
		}
		if err := igwrepo.NewIntegrationGatewayAPIKeyRepository(tx).UpdateOwnerPolicy(ctx, tenant, keyID, EncodePluginOwners(owners), actor); err != nil {
			return err
		}
		if err := igwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(tx).ReplaceAll(ctx, keyID, grants); err != nil {
			return err
		}
		extra, _ := json.Marshal(map[string]any{"actor": actor, "previous_plugin_ids": previous.Owners(), "previous_binding_mode": previous.Mode, "plugin_ids": owners})
		_, err = igwrepo.NewIntegrationGatewayAPIKeyAuditLogRepository(tx).Create(ctx, &models.IntegrationGatewayAPIKeyAuditLog{APIKeyUUID: keyID, TenantUUID: tenant, Path: "core://integration/api-key/plugin-owners", Method: "UPDATE", StatusCode: 200, Result: "success", TraceID: reqctx.GetTraceID(ctx), RequestExtra: datatypes.JSON(extra)})
		return err
	})
	if err != nil {
		return nil, err
	}
	_ = apikeycache.InvalidateAll(ctx)
	return s.GetOwners(ctx, tenant, keyID)
}

func (s *APIKeyOwnerService) mapKeyError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ownerError(dto.NewNotFound("当前租户未找到API Key或Profile", nil), "API_KEY_NOT_FOUND")
	}
	return err
}
