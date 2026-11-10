package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/pluginpermission"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

func (a pluginIAMAuthorizer) PermissionsForRoute(ctx context.Context, claims reqctx.CoreXClaims, pluginID, method, reqPath string) ([]string, string, error) {
	isAPIKey := false
	for _, platform := range claims.Platforms {
		isAPIKey = isAPIKey || platform == "api_key"
	}
	if !isAPIKey && reqctx.AuthenticatedAPIKeyHash(ctx) == "" {
		return a.PermissionsForClaims(ctx, claims, pluginID)
	}
	// API-key Profile IDs are not IAM member IDs. Never resolve these claims
	// through user roles, including on a cache hit in authentication middleware.
	if a.db == nil || !isAPIKey || reqctx.AuthenticatedAPIKeyHash(ctx) == "" || strings.TrimSpace(pluginID) == "" {
		return nil, "", fmt.Errorf("plugin API-key identity unavailable")
	}
	tenant, err := reqctx.CanonicalTenantUUID(claims.TenantUUID)
	if err != nil || tenant != reqctx.GetTenantUUID(ctx) {
		return nil, "", fmt.Errorf("plugin API-key tenant mismatch")
	}
	key, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(a.db).FindActiveByHash(ctx, tenant, reqctx.AuthenticatedAPIKeyHash(ctx))
	if err != nil {
		return nil, "", err
	}
	if key == nil || (key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now())) {
		return nil, "", fmt.Errorf("plugin API-key inactive")
	}
	profile, err := iamrepo.NewAPIKeyProfileRepository(a.db).GetById(ctx, key.ProfileID, nil)
	if err != nil {
		return nil, "", err
	}
	if profile == nil || profile.Status != 1 || profile.TenantUUID != tenant {
		return nil, "", fmt.Errorf("plugin API-key profile inactive")
	}
	permissions, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(a.db).ListByAPIKeyUUID(ctx, key.UUID)
	if err != nil {
		return nil, "", err
	}
	var definitions []iammodel.Permission
	if err := a.db.WithContext(ctx).Where("source = ? AND status = ? AND effect = ? AND allow_api_key = ?", "plugin:"+pluginID, iammodel.PermissionStatusActive, "allow", true).Find(&definitions).Error; err != nil {
		return nil, "", err
	}
	perms := []string{}
	for _, definition := range definitions {
		var meta map[string]any
		if json.Unmarshal(definition.Meta, &meta) != nil || !anyPluginAuthzBool(meta["api_key_explicit"]) || !pluginPermissionMetaMatchesServiceRoute(meta, method, reqPath) {
			continue
		}
		var declaration struct {
			CapabilityID string                      `json:"capability_id"`
			APIKey       pluginpermission.APIKeySpec `json:"api_key"`
		}
		if json.Unmarshal(definition.Meta, &declaration) != nil {
			continue
		}
		if declaration.APIKey.CapabilityID != "" && declaration.APIKey.CapabilityID != declaration.CapabilityID {
			continue
		}
		declaration.APIKey.CapabilityID = declaration.CapabilityID
		spec := declaration.APIKey
		if _, err := pluginpermission.ExplicitAPIKeyMetadata(&spec, pluginID, declaration.CapabilityID, true); err != nil {
			continue
		}
		if !gwrepo.APIKeyPermissionGrantedForPlugin(permissions, pluginID, spec.Scope, spec.Action, spec.ResourceType, spec.ResourcePattern) {
			continue
		}
		var published int64
		if err := a.db.WithContext(ctx).Model(&capmodels.CapabilityRecord{}).Where("capability_id = ? AND plugin_id = ? AND status = ?", spec.CapabilityID, pluginID, "published").Count(&published).Error; err != nil {
			return nil, "", err
		}
		if published == 0 {
			continue
		}
		var registrations []capmodels.CapabilityRegistration
		if err := a.db.WithContext(ctx).Where("tenant_uuid = ? AND capability_id = ?", tenant, spec.CapabilityID).Order("version DESC").Limit(1).Find(&registrations).Error; err != nil {
			return nil, "", err
		}
		if len(registrations) != 1 || registrations[0].Status != "published" {
			continue
		}
		if code := effectivePermissionCodeFromIAMRow(definition); code != "" {
			perms = append(perms, code)
		}
	}
	return dedupeSortedPluginPerms(perms), "", nil
}

func pluginPermissionMetaMatchesServiceRoute(meta map[string]any, method, reqPath string) bool {
	bindings, _ := meta["protocol_bindings"].([]any)
	for _, raw := range bindings {
		binding, _ := raw.(map[string]any)
		if binding == nil || anyPluginAuthzString(binding["actor_context"]) != "service_actor" || anyPluginAuthzString(binding["resource_scope"]) != "tenant" {
			continue
		}
		if pluginPermissionMetaMatchesRoute(map[string]any{"protocol_bindings": []any{binding}}, normalizePluginRouteMethod(method), normalizePluginRoutePath(reqPath)) {
			return true
		}
	}
	return false
}
