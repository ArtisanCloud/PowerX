package apikeypermissions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	apikeycache "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeycache"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const MaxBatchGrantKeys = 50

type BatchExplicitGrantInput struct {
	KeyIDs           []uuid.UUID
	PermissionIDs    []uint64
	DryRun           bool
	ExpectedPlanHash string
}

type BatchExplicitGrantKey struct {
	KeyID          string   `json:"key_id"`
	Name           string   `json:"name"`
	OwnerPluginIDs []string `json:"owner_plugin_ids"`
	AddedCount     int      `json:"added_count"`
	ExistingCount  int      `json:"existing_count"`
}

type BatchExplicitGrantPermission struct {
	ExplicitGrant
	Name string `json:"name"`
}

type BatchExplicitGrantResult struct {
	Committed   bool                           `json:"committed"`
	KeyCount    int                            `json:"key_count"`
	AddedCount  int                            `json:"added_count"`
	PlanHash    string                         `json:"plan_hash"`
	Keys        []BatchExplicitGrantKey        `json:"keys"`
	Permissions []BatchExplicitGrantPermission `json:"permissions"`
}

func batchGrantInvalid() error {
	return ownerError(dto.NewBadRequest("请选择 1 至 50 个 Key 和 1 至 128 项服务能力", nil), "API_KEY_BATCH_GRANT_INVALID")
}

// BatchAppendExplicitGrants 仅追加独立授权；预览与提交共用相同的租户、发布及资源校验。
func (s *APIKeyOwnerService) BatchAppendExplicitGrants(ctx context.Context, tenant string, input BatchExplicitGrantInput, actor string) (*BatchExplicitGrantResult, error) {
	if err := RequireOwnerAdministrator(ctx, tenant); err != nil {
		return nil, err
	}
	if len(input.KeyIDs) == 0 || len(input.KeyIDs) > MaxBatchGrantKeys || len(input.PermissionIDs) == 0 || len(input.PermissionIDs) > 128 {
		return nil, batchGrantInvalid()
	}
	keySet := map[uuid.UUID]bool{}
	keys := []uuid.UUID{}
	for _, id := range input.KeyIDs {
		if id == uuid.Nil {
			return nil, batchGrantInvalid()
		}
		if !keySet[id] {
			keySet[id] = true
			keys = append(keys, id)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	permissionSet := map[uint64]bool{}
	ids := []uint64{}
	for _, id := range input.PermissionIDs {
		if id == 0 {
			return nil, batchGrantInvalid()
		}
		if !permissionSet[id] {
			permissionSet[id] = true
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if !input.DryRun && strings.TrimSpace(input.ExpectedPlanHash) == "" {
		return nil, ownerError(dto.NewBadRequest("请先预览核对本次授权", nil), "API_KEY_BATCH_PREVIEW_REQUIRED")
	}
	result := &BatchExplicitGrantResult{Keys: []BatchExplicitGrantKey{}, Permissions: []BatchExplicitGrantPermission{}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		keyRepo := gwrepo.NewIntegrationGatewayAPIKeyRepository(tx)
		profileSet := map[uint64]bool{}
		profiles := []uint64{}
		originalProfiles := map[uuid.UUID]uint64{}
		keyNames := map[uuid.UUID]string{}
		// 先确定并按序锁定全部 Profile，再按 UUID 锁 Key，与单 Key 路径保持一致。
		for _, id := range keys {
			key, err := keyRepo.GetByTenantUUID(ctx, tenant, id, false)
			if err != nil {
				return dto.WithDetails(s.mapKeyError(err), map[string]any{"failed_key_id": id.String()})
			}
			originalProfiles[id] = key.ProfileID
			keyNames[id] = key.Name
			if !profileSet[key.ProfileID] {
				profileSet[key.ProfileID] = true
				profiles = append(profiles, key.ProfileID)
			}
		}
		sort.Slice(profiles, func(i, j int) bool { return profiles[i] < profiles[j] })
		for _, id := range profiles {
			profile, err := iamrepo.NewAPIKeyProfileRepository(tx).LockTenantProfile(ctx, tenant, id)
			if err != nil {
				return s.mapKeyError(err)
			}
			if profile.Status != 1 {
				return dto.WithDetails(ownerError(dto.NewConflict("所选 Key 的 Profile 已停用", nil), "API_KEY_PROFILE_INACTIVE"), map[string]any{"failed_profile_id": id, "failed_profile_name": profile.Name})
			}
		}
		canonical := []ExplicitGrant{}
		for _, id := range ids {
			grant, err := canonicalExplicitGrant(ctx, tx, tenant, id)
			var permission iammodel.Permission
			name := ""
			if readErr := tx.WithContext(ctx).Select("id", "resource", "action", "meta").First(&permission, id).Error; readErr == nil {
				name = strings.TrimSpace(anyToString(parseMeta(permission.Meta)["label"]))
				if name == "" {
					name = permission.Resource + "." + permission.Action
				}
			}
			if err != nil {
				return dto.WithDetails(err, map[string]any{"failed_permission_id": id, "failed_permission_name": name})
			}
			canonical = append(canonical, grant)
			result.Permissions = append(result.Permissions, BatchExplicitGrantPermission{ExplicitGrant: grant, Name: name})
		}
		plans := []*explicitGrantChange{}
		for _, id := range keys {
			plan, err := s.prepareExplicitGrantChangeTx(ctx, tx, tenant, id, func(_ *gorm.DB, policy *OwnerPolicy) error {
				for _, grant := range canonical {
					found := false
					for i := range policy.ExplicitGrants {
						if policy.ExplicitGrants[i].PermissionID == grant.PermissionID {
							policy.ExplicitGrants[i] = grant
							found = true
							break
						}
					}
					if !found {
						policy.ExplicitGrants = append(policy.ExplicitGrants, grant)
					}
				}
				sort.Slice(policy.ExplicitGrants, func(i, j int) bool {
					return policy.ExplicitGrants[i].PermissionID < policy.ExplicitGrants[j].PermissionID
				})
				return validateStoredExplicitGrants(policy.ExplicitGrants)
			})
			if err != nil {
				return dto.WithDetails(err, map[string]any{"failed_key_id": id.String(), "failed_key_name": keyNames[id]})
			}
			if plan.key.ProfileID != originalProfiles[id] {
				return ownerError(dto.NewConflict("Key 所属 Profile 已变化，请重新预览", nil), "API_KEY_BATCH_PREVIEW_STALE")
			}
			previous := []ExplicitGrant{}
			if err := json.Unmarshal(plan.before, &previous); err != nil {
				return err
			}
			existing := map[uint64]bool{}
			for _, grant := range previous {
				existing[grant.PermissionID] = true
			}
			added := 0
			for _, grant := range canonical {
				if !existing[grant.PermissionID] {
					added++
				}
			}
			result.Keys = append(result.Keys, BatchExplicitGrantKey{KeyID: id.String(), Name: plan.key.Name, OwnerPluginIDs: plan.policy.Owners(), AddedCount: added, ExistingCount: len(canonical) - added})
			result.AddedCount += added
			plans = append(plans, plan)
		}
		hash, err := batchGrantPlanHash(tenant, canonical, plans)
		if err != nil {
			return err
		}
		result.PlanHash = hash
		result.KeyCount = len(plans)
		if input.DryRun {
			return nil
		}
		if input.ExpectedPlanHash != hash {
			return ownerError(dto.NewConflict("预览后授权配置已变化，请重新预览", nil), "API_KEY_BATCH_PREVIEW_STALE")
		}
		for _, plan := range plans {
			if err := s.persistExplicitGrantChangeTx(ctx, tx, tenant, actor, plan); err != nil {
				return dto.WithDetails(err, map[string]any{"failed_key_id": plan.key.UUID.String(), "failed_key_name": plan.key.Name})
			}
			// 在提交前回读独立声明，确认实际写入，不以计划结果代替保存结果。
			stored, err := keyRepo.GetByTenantUUID(ctx, tenant, plan.key.UUID, false)
			if err != nil {
				return err
			}
			policy, err := ResolveOwnerPolicy(stored.PluginOwnerPolicy, nil)
			if err != nil {
				return err
			}
			for _, wanted := range canonical {
				found := false
				for _, saved := range policy.ExplicitGrants {
					if saved == wanted {
						found = true
						break
					}
				}
				if !found {
					return errors.New("batch grant readback mismatch")
				}
			}
		}
		result.Committed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result.Committed {
		_ = apikeycache.InvalidateAll(ctx)
	}
	return result, nil
}

func batchGrantPlanHash(tenant string, grants []ExplicitGrant, plans []*explicitGrantChange) (string, error) {
	type keySnapshot struct {
		ID        string
		Name      string
		ProfileID uint64
		Policy    json.RawMessage
		Grants    []string
	}
	snapshots := []keySnapshot{}
	for _, plan := range plans {
		tuples := []string{}
		for _, row := range plan.current {
			tuples = append(tuples, strings.Join([]string{row.PluginID, row.Scope, row.Action, row.ResourceType, row.ResourcePattern, row.Effect}, "\x00"))
		}
		sort.Strings(tuples)
		snapshots = append(snapshots, keySnapshot{ID: plan.key.UUID.String(), Name: plan.key.Name, ProfileID: plan.key.ProfileID, Policy: plan.original, Grants: tuples})
	}
	raw, err := json.Marshal(struct {
		Tenant      string
		Permissions []ExplicitGrant
		Keys        []keySnapshot
	}{tenant, grants, snapshots})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}
