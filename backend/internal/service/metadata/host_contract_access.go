package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
)

const (
	ReasonInvalidArgument    = "METADATA_INVALID_ARGUMENT"
	ReasonUnauthorized       = "METADATA_UNAUTHORIZED"
	ReasonForbidden          = "METADATA_FORBIDDEN"
	ReasonNotFound           = "METADATA_NOT_FOUND"
	ReasonConflict           = "METADATA_CONFLICT"
	ReasonUpstreamDependency = "METADATA_UPSTREAM_DEPENDENCY"
)

type HostContractAccess struct{ db *gorm.DB }

func NewHostContractAccess(db *gorm.DB) *HostContractAccess { return &HostContractAccess{db: db} }

type hostCredential struct {
	AllowedCapabilities []string `json:"allowed_capabilities"`
}

func (a *HostContractAccess) Authorize(ctx context.Context, apiKeyHash, resource, action string) (string, error) {
	if a == nil || a.db == nil {
		return "", upstream(errors.New("metadata authorization unavailable"))
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil {
		return "", unauthorized(errors.New("service actor missing"))
	}
	tenantUUID, err := reqctx.RequireTenantUUID(ctx)
	if err != nil {
		return "", unauthorized(err)
	}
	tenantUUID, err = reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return "", unauthorized(err)
	}
	capabilityID := "com.corex.metadata." + resource + "." + action
	if err = a.requirePublished(ctx, tenantUUID, capabilityID); err != nil {
		return "", err
	}
	if contains(claims.Platforms, "api_key") {
		if strings.TrimSpace(apiKeyHash) == "" {
			return "", unauthorized(errors.New("api key identity missing"))
		}
		key, e := gwrepo.NewIntegrationGatewayAPIKeyRepository(a.db).FindActiveByHash(ctx, tenantUUID, apiKeyHash)
		if e != nil || key == nil {
			if e != nil {
				return "", upstream(e)
			}
			return "", unauthorized(errors.New("api key identity missing"))
		}
		ok, e := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(a.db).HasPermission(ctx, key.UUID, "_scope.metadata."+resource+"."+action, action, "api", resource)
		if e != nil {
			return "", upstream(e)
		}
		if !ok {
			return "", forbidden(errors.New("api key capability grant missing"))
		}
		return tenantUUID, nil
	}
	if !strings.EqualFold(strings.TrimSpace(claims.Issuer), "powerx-sts") || !contains(claims.Audience, "powerx:api") || strings.TrimSpace(claims.PluginID) == "" {
		return "", unauthorized(errors.New("invalid sts service actor"))
	}
	var credential dbsetting.PluginInstanceConfig
	if err = a.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", tenantUUID, strings.TrimSpace(claims.PluginID), settingrepo.KeyClientCredentials, true).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", forbidden(errors.New("plugin capability grant missing"))
		}
		return "", upstream(err)
	}
	var value hostCredential
	if err = json.Unmarshal(credential.ValueJSON, &value); err != nil {
		return "", upstream(err)
	}
	if !contains(value.AllowedCapabilities, capabilityID) {
		return "", forbidden(errors.New("plugin capability grant missing"))
	}
	return tenantUUID, nil
}
func (a *HostContractAccess) requirePublished(ctx context.Context, tenantUUID, capabilityID string) error {
	var capability capmodels.CapabilityRecord
	if err := a.db.WithContext(ctx).Where("capability_id = ? AND status = ?", capabilityID, "published").First(&capability).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return forbidden(errors.New("capability not published"))
		}
		return upstream(err)
	}
	var registration capmodels.CapabilityRegistration
	if err := a.db.WithContext(ctx).Where("capability_id = ? AND tenant_uuid = ? AND status = ?", capabilityID, tenantUUID, "published").Order("version DESC").First(&registration).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return forbidden(errors.New("tenant capability registration missing"))
		}
		return upstream(err)
	}
	return nil
}
func contains(values []string, target string) bool {
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), target) {
			return true
		}
	}
	return false
}
func hostError(status int, reason string, err error) error {
	return dto.NewErrorWithCode(status, reason, reason, err)
}
func invalid(err error) error             { return hostError(http.StatusBadRequest, ReasonInvalidArgument, err) }
func HostInvalidArgument(err error) error { return invalid(err) }
func unauthorized(err error) error {
	return hostError(http.StatusUnauthorized, ReasonUnauthorized, err)
}
func forbidden(err error) error    { return hostError(http.StatusForbidden, ReasonForbidden, err) }
func notFound(err error) error     { return hostError(http.StatusNotFound, ReasonNotFound, err) }
func HostNotFound(err error) error { return notFound(err) }
func conflict(err error) error     { return hostError(http.StatusConflict, ReasonConflict, err) }
func HostConflict(err error) error { return conflict(err) }
func upstream(err error) error {
	return hostError(http.StatusServiceUnavailable, ReasonUpstreamDependency, err)
}
func HostUpstreamDependency(err error) error { return upstream(err) }
