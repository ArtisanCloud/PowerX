package apikeypermissions

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"

	apikeycache "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeycache"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	igwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/pluginpermission"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ExplicitGrant struct {
	PermissionID uint64 `json:"permission_id"`
	CapabilityID string `json:"capability_id"`
	OwnerPermissionBinding
}

type ExplicitGrantResult struct {
	KeyID      string          `json:"key_id"`
	TenantUUID string          `json:"tenant_uuid"`
	Grants     []ExplicitGrant `json:"grants"`
}

func invalidExplicitGrant(cause error) error {
	return ownerError(dto.NewBadRequest("API_KEY_EXPLICIT_GRANT_INVALID", cause), "API_KEY_EXPLICIT_GRANT_INVALID")
}

func validateStoredExplicitGrants(grants []ExplicitGrant) error {
	if len(grants) > 128 {
		return invalidExplicitGrant(nil)
	}
	seen := map[uint64]bool{}
	for _, g := range grants {
		if g.PermissionID == 0 || seen[g.PermissionID] {
			return invalidExplicitGrant(nil)
		}
		if err := validateExplicitGrantSpec(g); err != nil {
			return err
		}
		seen[g.PermissionID] = true
	}
	return nil
}

func validateExplicitGrantSpec(g ExplicitGrant) error {
	if g.PluginID == "" && strings.HasPrefix(g.CapabilityID, "com.corex.") {
		if !strings.HasPrefix(g.Scope, "_scope.") || g.Effect != "allow" || (g.ResourceType != "api" && g.ResourceType != "capability") {
			return invalidExplicitGrant(nil)
		}
		for _, value := range []string{g.CapabilityID, g.Scope, g.Action, g.ResourceType, g.ResourcePattern} {
			if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "*\x00\r\n\t ") {
				return invalidExplicitGrant(nil)
			}
		}
		return nil
	}
	if !exactPluginID.MatchString(g.PluginID) {
		return invalidExplicitGrant(nil)
	}
	spec := &pluginpermission.APIKeySpec{CapabilityID: g.CapabilityID, PluginID: g.PluginID, Scope: g.Scope, Action: g.Action, ResourceType: g.ResourceType, ResourcePattern: g.ResourcePattern, Effect: g.Effect}
	if _, err := pluginpermission.ExplicitAPIKeyMetadata(spec, g.PluginID, g.CapabilityID, true); err != nil {
		return invalidExplicitGrant(err)
	}
	return nil
}

func mergeExplicitGrantRows(keyID uuid.UUID, base []models.IntegrationGatewayAPIKeyPermission, grants []ExplicitGrant) []models.IntegrationGatewayAPIKeyPermission {
	seen := map[string]bool{}
	out := make([]models.IntegrationGatewayAPIKeyPermission, 0, len(base)+len(grants))
	add := func(row models.IntegrationGatewayAPIKeyPermission) {
		key := strings.Join([]string{row.PluginID, row.Scope, row.Action, row.ResourceType, row.ResourcePattern, row.Effect}, "\x00")
		if !seen[key] {
			seen[key] = true
			out = append(out, models.IntegrationGatewayAPIKeyPermission{APIKeyUUID: keyID, PluginID: row.PluginID, Scope: row.Scope, Action: row.Action, ResourceType: row.ResourceType, ResourcePattern: row.ResourcePattern, Effect: row.Effect})
		}
	}
	for _, row := range base {
		add(row)
	}
	for _, g := range grants {
		add(models.IntegrationGatewayAPIKeyPermission{APIKeyUUID: keyID, PluginID: g.PluginID, Scope: g.Scope, Action: g.Action, ResourceType: g.ResourceType, ResourcePattern: g.ResourcePattern, Effect: g.Effect})
	}
	return out
}

func (s *APIKeyOwnerService) GetExplicitGrants(ctx context.Context, tenant string, keyID uuid.UUID) (*ExplicitGrantResult, error) {
	if err := RequireOwnerAdministrator(ctx, tenant); err != nil {
		return nil, err
	}
	key, err := igwrepo.NewIntegrationGatewayAPIKeyRepository(s.db).GetByTenantUUID(ctx, tenant, keyID, false)
	if err != nil {
		return nil, s.mapKeyError(err)
	}
	rows, err := igwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(s.db).ListByAPIKeyUUID(ctx, keyID)
	if err != nil {
		return nil, err
	}
	policy, err := ResolveOwnerPolicy(key.PluginOwnerPolicy, rows)
	if err != nil {
		return nil, err
	}
	grants := policy.ExplicitGrants
	if grants == nil {
		grants = []ExplicitGrant{}
	}
	return &ExplicitGrantResult{KeyID: keyID.String(), TenantUUID: tenant, Grants: grants}, nil
}

// Grant inputs are canonical IAM permission IDs, never arbitrary scope strings.
// The permission declaration, publication and latest tenant registration must
// all be valid before any part of the Key policy is changed.
func (s *APIKeyOwnerService) AppendExplicitGrants(ctx context.Context, tenant string, keyID uuid.UUID, permissionIDs []uint64, actor string) (*ExplicitGrantResult, error) {
	if len(permissionIDs) == 0 || len(permissionIDs) > 128 {
		return nil, invalidExplicitGrant(nil)
	}
	return s.changeExplicitGrants(ctx, tenant, keyID, actor, func(tx *gorm.DB, policy *OwnerPolicy) error {
		for _, id := range permissionIDs {
			g, err := canonicalExplicitGrant(ctx, tx, tenant, id)
			if err != nil {
				return err
			}
			found := false
			for i := range policy.ExplicitGrants {
				if policy.ExplicitGrants[i].PermissionID == id {
					policy.ExplicitGrants[i], found = g, true
					break
				}
			}
			if !found {
				policy.ExplicitGrants = append(policy.ExplicitGrants, g)
			}
		}
		sort.Slice(policy.ExplicitGrants, func(i, j int) bool {
			return policy.ExplicitGrants[i].PermissionID < policy.ExplicitGrants[j].PermissionID
		})
		return validateStoredExplicitGrants(policy.ExplicitGrants)
	})
}

func (s *APIKeyOwnerService) RevokeExplicitGrant(ctx context.Context, tenant string, keyID uuid.UUID, permissionID uint64, actor string) (*ExplicitGrantResult, error) {
	if permissionID == 0 {
		return nil, invalidExplicitGrant(nil)
	}
	return s.changeExplicitGrants(ctx, tenant, keyID, actor, func(_ *gorm.DB, policy *OwnerPolicy) error {
		kept := []ExplicitGrant{}
		for _, grant := range policy.ExplicitGrants {
			if grant.PermissionID != permissionID {
				kept = append(kept, grant)
			}
		}
		policy.ExplicitGrants = kept
		return nil
	})
}

func canonicalExplicitGrant(ctx context.Context, tx *gorm.DB, tenant string, permissionID uint64) (ExplicitGrant, error) {
	if permissionID == 0 {
		return ExplicitGrant{}, invalidExplicitGrant(nil)
	}
	var p iammodel.Permission
	if err := tx.WithContext(ctx).First(&p, permissionID).Error; err != nil {
		return ExplicitGrant{}, invalidExplicitGrant(err)
	}
	var meta struct {
		CapabilityID string                      `json:"capability_id"`
		Explicit     bool                        `json:"api_key_explicit"`
		APIKey       pluginpermission.APIKeySpec `json:"api_key"`
		Type         string                      `json:"type"`
		CoreEndpoint string                      `json:"core_internal_endpoint"`
	}
	if json.Unmarshal(p.Meta, &meta) != nil || !p.AllowAPIKey || p.Status != iammodel.PermissionStatusActive || p.Effect != "allow" || !meta.Explicit {
		return ExplicitGrant{}, invalidExplicitGrant(nil)
	}
	if meta.APIKey.CapabilityID != "" && meta.APIKey.CapabilityID != meta.CapabilityID {
		return ExplicitGrant{}, invalidExplicitGrant(nil)
	}
	providerID := ""
	if strings.HasPrefix(p.Source, "plugin:") {
		providerID = strings.TrimPrefix(p.Source, "plugin:")
		if providerID == "" || meta.APIKey.PluginID != providerID {
			return ExplicitGrant{}, invalidExplicitGrant(nil)
		}
	} else if p.Source == platformPermissionSource && meta.Type == "capability_service" && meta.APIKey.PluginID == "" && strings.HasPrefix(meta.CapabilityID, "com.corex.") {
		providerID = "corex.platform"
	} else {
		return ExplicitGrant{}, invalidExplicitGrant(nil)
	}
	grant := ExplicitGrant{PermissionID: p.ID, CapabilityID: meta.CapabilityID, OwnerPermissionBinding: OwnerPermissionBinding{Scope: meta.APIKey.Scope, Action: meta.APIKey.Action, ResourceType: meta.APIKey.ResourceType, ResourcePattern: meta.APIKey.ResourcePattern, PluginID: meta.APIKey.PluginID, Effect: meta.APIKey.Effect}}
	if err := validateExplicitGrantSpec(grant); err != nil {
		return ExplicitGrant{}, err
	}
	var record capmodels.CapabilityRecord
	if err := tx.WithContext(ctx).Where("capability_id = ? AND plugin_id = ? AND status = ?", meta.CapabilityID, providerID, "published").First(&record).Error; err != nil {
		return ExplicitGrant{}, ownerError(dto.NewConflict("CAPABILITY_UNKNOWN", err), "CAPABILITY_UNKNOWN")
	}
	if providerID == "corex.platform" {
		u, err := url.Parse(meta.CoreEndpoint)
		if err != nil || u.Scheme != "core" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return ExplicitGrant{}, invalidExplicitGrant(nil)
		}
		bound := false
		var protocols []capmodels.ProtocolBinding
		if json.Unmarshal(record.Protocols, &protocols) != nil {
			return ExplicitGrant{}, invalidExplicitGrant(nil)
		}
		for _, protocol := range protocols {
			bound = bound || (protocol.Channel == "core_internal" && protocol.Method == "INVOKE" && protocol.Endpoint == meta.CoreEndpoint && protocol.ActorContext == "service_actor" && protocol.ResourceScope == "tenant")
		}
		if !bound {
			return ExplicitGrant{}, invalidExplicitGrant(nil)
		}
	}
	var reg capmodels.CapabilityRegistration
	if err := tx.WithContext(ctx).Where("tenant_uuid = ? AND capability_id = ?", tenant, meta.CapabilityID).Order("version DESC").First(&reg).Error; err != nil || reg.Status != "published" {
		return ExplicitGrant{}, ownerError(dto.NewConflict("CAPABILITY_TENANT_NOT_REGISTERED", err), "CAPABILITY_TENANT_NOT_REGISTERED")
	}
	return grant, nil
}

type explicitGrantChange struct {
	key      *models.IntegrationGatewayAPIKey
	policy   *OwnerPolicy
	rows     []models.IntegrationGatewayAPIKeyPermission
	before   json.RawMessage
	original json.RawMessage
	current  []models.IntegrationGatewayAPIKeyPermission
}

func (s *APIKeyOwnerService) prepareExplicitGrantChangeTx(ctx context.Context, tx *gorm.DB, tenant string, keyID uuid.UUID, change func(*gorm.DB, *OwnerPolicy) error) (*explicitGrantChange, error) {
	key, policy, err := s.SnapshotKeyTx(ctx, tx, tenant, keyID)
	if err != nil {
		return nil, err
	}
	before, _ := json.Marshal(policy.ExplicitGrants)
	original, _ := json.Marshal(policy)
	previousGrants := append([]ExplicitGrant(nil), policy.ExplicitGrants...)
	current, err := igwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(tx).ListByAPIKeyUUID(ctx, keyID)
	if err != nil {
		return nil, err
	}
	if err := change(tx, policy); err != nil {
		return nil, err
	}
	// A Key-only mutation must not reconcile unrelated Profile changes.
	// Retain existing effective rows; only replace previously managed grants.
	inherited := []models.IntegrationGatewayAPIKeyPermission{}
	if len(previousGrants) > 0 {
		inheritedPolicy := *policy
		inheritedPolicy.ExplicitGrants = nil
		inherited, err = s.BindProfileGrants(ctx, tx, tenant, key.ProfileID, keyID, inheritedPolicy)
		if err != nil {
			return nil, err
		}
	}
	kept := []models.IntegrationGatewayAPIKeyPermission{}
	for _, row := range current {
		managed := false
		for _, grant := range previousGrants {
			if row.PluginID == grant.PluginID && row.Scope == grant.Scope && row.Action == grant.Action && row.ResourceType == grant.ResourceType && row.ResourcePattern == grant.ResourcePattern && row.Effect == grant.Effect {
				managed = true
				break
			}
		}
		if managed {
			for _, base := range inherited {
				if row.PluginID == base.PluginID && row.Scope == base.Scope && row.Action == base.Action && row.ResourceType == base.ResourceType && row.ResourcePattern == base.ResourcePattern && row.Effect == base.Effect {
					managed = false
					break
				}
			}
		}
		if !managed {
			kept = append(kept, row)
		}
	}
	rows := mergeExplicitGrantRows(keyID, kept, policy.ExplicitGrants)
	return &explicitGrantChange{key: key, policy: policy, rows: rows, before: before, original: original, current: current}, nil
}

func (s *APIKeyOwnerService) persistExplicitGrantChangeTx(ctx context.Context, tx *gorm.DB, tenant, actor string, plan *explicitGrantChange) error {
	if err := igwrepo.NewIntegrationGatewayAPIKeyRepository(tx).UpdateOwnerPolicy(ctx, tenant, plan.key.UUID, plan.policy.Encode(), actor); err != nil {
		return err
	}
	if err := igwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(tx).ReplaceAll(ctx, plan.key.UUID, plan.rows); err != nil {
		return err
	}
	extra, _ := json.Marshal(map[string]any{"actor": actor, "before": json.RawMessage(plan.before), "after": plan.policy.ExplicitGrants})
	_, err := igwrepo.NewIntegrationGatewayAPIKeyAuditLogRepository(tx).Create(ctx, &models.IntegrationGatewayAPIKeyAuditLog{APIKeyUUID: plan.key.UUID, TenantUUID: tenant, Path: "core://integration/api-key/explicit-grants", Method: "UPDATE", StatusCode: 200, Result: "success", TraceID: reqctx.GetTraceID(ctx), RequestExtra: datatypes.JSON(extra)})
	return err
}

func (s *APIKeyOwnerService) changeExplicitGrants(ctx context.Context, tenant string, keyID uuid.UUID, actor string, change func(*gorm.DB, *OwnerPolicy) error) (*ExplicitGrantResult, error) {
	if err := RequireOwnerAdministrator(ctx, tenant); err != nil {
		return nil, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		plan, err := s.prepareExplicitGrantChangeTx(ctx, tx, tenant, keyID, change)
		if err != nil {
			return err
		}
		return s.persistExplicitGrantChangeTx(ctx, tx, tenant, actor, plan)
	})
	if err != nil {
		return nil, err
	}
	_ = apikeycache.InvalidateAll(ctx)
	return s.GetExplicitGrants(ctx, tenant, keyID)
}
