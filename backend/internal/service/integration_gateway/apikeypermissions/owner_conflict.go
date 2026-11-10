package apikeypermissions

import (
	"context"
	"fmt"
	"sort"
	"strings"

	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
)

// OwnerPermissionConflict 仅包含管理诊断需要的标识，不返回 Key 明文、前缀或哈希。
type OwnerPermissionConflict struct {
	KeyID                    string   `json:"key_id"`
	KeyName                  string   `json:"key_name"`
	BindingMode              string   `json:"binding_mode"`
	OwnerPluginIDs           []string `json:"owner_plugin_ids"`
	PermissionOwnerPluginIDs []string `json:"permission_owner_plugin_ids"`
	PermissionID             uint64   `json:"permission_id"`
	PermissionName           string   `json:"permission_name"`
	CapabilityID             string   `json:"capability_id,omitempty"`
	RequiredPluginID         string   `json:"required_plugin_id"`
	Scope                    string   `json:"scope"`
	Action                   string   `json:"action"`
	ResourceType             string   `json:"resource_type"`
	ResourcePattern          string   `json:"resource_pattern"`
}

type profileGrantDefinition struct {
	permissionID   uint64
	permissionName string
	capabilityID   string
	grant          models.IntegrationGatewayAPIKeyPermission
	providerGrant  *ExplicitGrant
}

func (s *APIKeyOwnerService) profileGrantDefinitions(ctx context.Context, tx *gorm.DB, profileID uint64) ([]profileGrantDefinition, error) {
	ids, err := iamrepo.NewAPIKeyProfilePermissionRepository(tx).ListPermissionIDsOfProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	rows, err := iamrepo.NewPermissionRepository(tx).FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := []profileGrantDefinition{}
	for _, p := range rows {
		if p == nil || p.Status != iammodel.PermissionStatusActive || !p.AllowAPIKey {
			continue
		}
		meta := parseMeta(p.Meta)
		var providerGrant *ExplicitGrant
		// 这是“可调用哪项插件服务”，不能拿服务提供方去匹配调用方的资源 owner。
		// 仅接受完整显式服务声明；发布与当前租户注册仍需通过同一套正式校验。
		if strings.HasPrefix(p.Source, "plugin:") && meta["api_key_explicit"] == true {
			profile, err := iamrepo.NewAPIKeyProfileRepository(tx).GetById(ctx, profileID, nil)
			if err != nil {
				return nil, err
			}
			if profile == nil {
				return nil, dto.NewNotFound("API Key Profile不存在", nil)
			}
			grant, err := canonicalExplicitGrant(ctx, tx, profile.TenantUUID, p.ID)
			if err != nil {
				name := strings.TrimSpace(anyToString(meta["label"]))
				if name == "" {
					name = p.Resource + "." + p.Action
				}
				message := fmt.Sprintf("权限 %s (#%d)：%s", name, p.ID, dto.MessageOf(err))
				return nil, dto.WithDetails(dto.Wrap(err, message), map[string]any{"failed_permission_id": p.ID, "failed_permission_name": name, "failed_capability_id": meta["capability_id"]})
			}
			providerGrant = &grant
		}
		resolved, ok := ResolvePermission(*p)
		if !ok {
			continue
		}
		name := strings.TrimSpace(anyToString(meta["label"]))
		if name == "" {
			name = p.Resource + "." + p.Action
		}
		capabilityID := strings.TrimSpace(anyToString(meta["capability_id"]))
		if capabilityID == "" {
			if explicit, ok := meta["api_key"].(map[string]any); ok {
				capabilityID = strings.TrimSpace(anyToString(explicit["capability_id"]))
			}
		}
		out = append(out, profileGrantDefinition{permissionID: p.ID, permissionName: name, capabilityID: capabilityID, providerGrant: providerGrant, grant: models.IntegrationGatewayAPIKeyPermission{Scope: resolved.Scope, Action: resolved.Action, ResourceType: resolved.ResourceType, ResourcePattern: resolved.ResourcePattern, PluginID: resolved.PluginID, Effect: resolved.Effect}})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].permissionID < out[j].permissionID })
	return out, nil
}

func grantsOf(definitions []profileGrantDefinition) []models.IntegrationGatewayAPIKeyPermission {
	out := make([]models.IntegrationGatewayAPIKeyPermission, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, definition.grant)
	}
	return out
}

// 旧策略按完整权限元组计算 owner，不能用其他操作的 owner 扩大本项授权。
func ownersForGrant(policy OwnerPolicy, item models.IntegrationGatewayAPIKeyPermission) []string {
	if policy.Mode == "key" {
		return policy.PluginIDs
	}
	owners := []string{}
	for _, b := range policy.Bindings {
		if b.Scope == item.Scope && b.Action == item.Action && b.ResourceType == item.ResourceType && b.ResourcePattern == item.ResourcePattern && b.Effect == item.Effect && (item.PluginID == "" || item.PluginID == b.PluginID) {
			owners = append(owners, b.PluginID)
		}
	}
	return owners
}

const maxOwnerConflictDetails = 200

type ownerConflictReport struct {
	conflicts []OwnerPermissionConflict
	count     int
	keys      map[string]struct{}
}

func (r *ownerConflictReport) add(key models.IntegrationGatewayAPIKey, policy OwnerPolicy, definitions []profileGrantDefinition) {
	for _, definition := range definitions {
		item := definition.grant
		if definition.providerGrant != nil || item.PluginID == "" {
			continue
		}
		owners := ownersForGrant(policy, item)
		matched := false
		for _, owner := range owners {
			if owner == item.PluginID {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		r.count++
		if r.keys == nil {
			r.keys = map[string]struct{}{}
		}
		r.keys[key.UUID.String()] = struct{}{}
		if len(r.conflicts) >= maxOwnerConflictDetails {
			continue
		}
		configured := append([]string{}, policy.Owners()...)
		specific := append([]string{}, owners...)
		r.conflicts = append(r.conflicts, OwnerPermissionConflict{KeyID: key.UUID.String(), KeyName: key.Name, BindingMode: policy.Mode, OwnerPluginIDs: configured, PermissionOwnerPluginIDs: specific, PermissionID: definition.permissionID, PermissionName: definition.permissionName, CapabilityID: definition.capabilityID, RequiredPluginID: item.PluginID, Scope: item.Scope, Action: item.Action, ResourceType: item.ResourceType, ResourcePattern: item.ResourcePattern})
	}
}

func (r *ownerConflictReport) err(profileID uint64, profileName string) error {
	if r.count == 0 {
		return nil
	}
	err := ownerError(dto.NewBadRequest(fmt.Sprintf("发现 %d 项插件归属冲突，涉及 %d 个 API Key", r.count, len(r.keys)), nil), "API_KEY_PLUGIN_OWNER_CONFLICT")
	return dto.WithDetails(err, map[string]any{"profile_id": profileID, "profile_name": profileName, "conflict_count": r.count, "conflicting_key_count": len(r.keys), "conflicts": r.conflicts, "truncated": r.count > len(r.conflicts)})
}
