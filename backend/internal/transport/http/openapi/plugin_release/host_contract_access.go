package plugin_release

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
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	pluginReleaseSessionsReadCapabilityID   = "com.corex.plugin_release.sessions.read"
	pluginReleaseSessionsManageCapabilityID = "com.corex.plugin_release.sessions.manage"
	pluginReleaseImportsReadCapabilityID    = "com.corex.plugin_release.imports.read"
	pluginReleaseImportsManageCapabilityID  = "com.corex.plugin_release.imports.manage"
	pluginReleaseSessionsReadAPIKeyScope    = "_scope.plugin_release.sessions.read"
	pluginReleaseSessionsManageAPIKeyScope  = "_scope.plugin_release.sessions.manage"
	pluginReleaseImportsReadAPIKeyScope     = "_scope.plugin_release.imports.read"
	pluginReleaseImportsManageAPIKeyScope   = "_scope.plugin_release.imports.manage"
)

// hostContractAccess is deliberately route-local: it requires publication,
// tenant registration, and the credential's concrete grant. STS admission is
// not authorization.
type hostContractAccess struct{ db *gorm.DB }

func newHostContractAccess(db *gorm.DB) *hostContractAccess { return &hostContractAccess{db: db} }

type pluginReleaseCredential struct {
	AllowedCapabilities []string `json:"allowed_capabilities"`
}

func (a *hostContractAccess) authorize(ctx context.Context, apiKeyHash, capabilityID, apiKeyScope, resource, action string) (string, error) {
	if a == nil || a.db == nil {
		return "", pluginReleaseUpstream(errors.New("authorization unavailable"))
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil {
		return "", pluginReleaseUnauthorized(errors.New("service credential required"))
	}
	tenantUUID, err := reqctx.RequireTenantUUID(ctx)
	if err != nil {
		return "", pluginReleaseUnauthorized(err)
	}
	tenantUUID, err = reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return "", pluginReleaseUnauthorized(err)
	}
	if err := a.requirePublished(ctx, tenantUUID, capabilityID); err != nil {
		return "", err
	}
	if contains(claims.Platforms, "api_key") {
		key, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(a.db).FindActiveByHash(ctx, tenantUUID, strings.TrimSpace(apiKeyHash))
		if err != nil {
			return "", pluginReleaseUpstream(err)
		}
		if key == nil {
			return "", pluginReleaseUnauthorized(errors.New("api key identity missing"))
		}
		allowed, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(a.db).HasPermission(ctx, key.UUID, apiKeyScope, action, "api", resource)
		if err != nil {
			return "", pluginReleaseUpstream(err)
		}
		if !allowed {
			return "", pluginReleaseForbidden(errors.New("api key capability grant missing"))
		}
		return tenantUUID, nil
	}
	if !strings.EqualFold(strings.TrimSpace(claims.Issuer), "powerx-sts") || !contains(claims.Audience, "powerx:api") || strings.TrimSpace(claims.PluginID) == "" || strings.TrimSpace(claims.Subject) == "" {
		return "", pluginReleaseUnauthorized(errors.New("invalid sts service actor"))
	}
	var credential dbsetting.PluginInstanceConfig
	if err := a.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", tenantUUID, claims.PluginID, settingrepo.KeyClientCredentials, true).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", pluginReleaseForbidden(errors.New("plugin capability grant missing"))
		}
		return "", pluginReleaseUpstream(err)
	}
	var value pluginReleaseCredential
	if err := json.Unmarshal(credential.ValueJSON, &value); err != nil {
		return "", pluginReleaseUpstream(err)
	}
	if !contains(value.AllowedCapabilities, capabilityID) {
		return "", pluginReleaseForbidden(errors.New("plugin capability grant missing"))
	}
	return tenantUUID, nil
}

func (a *hostContractAccess) requirePublished(ctx context.Context, tenantUUID, capabilityID string) error {
	var cap capmodels.CapabilityRecord
	if err := a.db.WithContext(ctx).Where("capability_id = ? AND status = ?", capabilityID, "published").First(&cap).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return pluginReleaseForbidden(errors.New("capability is not published"))
		}
		return pluginReleaseUpstream(err)
	}
	var registration capmodels.CapabilityRegistration
	if err := a.db.WithContext(ctx).Where("capability_id = ? AND tenant_uuid = ? AND status = ?", capabilityID, tenantUUID, "published").First(&registration).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return pluginReleaseForbidden(errors.New("tenant capability registration missing"))
		}
		return pluginReleaseUpstream(err)
	}
	return nil
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), want) {
			return true
		}
	}
	return false
}
func hostAPIKeyHash(c *gin.Context) string {
	v, _ := c.Get("auth_api_key_hash")
	hash, _ := v.(string)
	return strings.TrimSpace(hash)
}
func rejectTenantOverride(c *gin.Context) bool {
	if strings.TrimSpace(c.Query("tenant_uuid")) != "" || strings.TrimSpace(c.GetHeader("X-Tenant-UUID")) != "" {
		dto.RespondErrorFrom(c, pluginReleaseInvalidArgument(errors.New("tenant override must not be supplied")))
		return false
	}
	return true
}
func pluginReleaseError(status int, code string, err error) error {
	return dto.NewErrorWithCode(status, code, code, err)
}
func pluginReleaseInvalidArgument(err error) error {
	return pluginReleaseError(http.StatusBadRequest, "PLUGIN_RELEASE_INVALID_ARGUMENT", err)
}
func pluginReleaseUnauthorized(err error) error {
	return pluginReleaseError(http.StatusUnauthorized, "PLUGIN_RELEASE_UNAUTHORIZED", err)
}
func pluginReleaseForbidden(err error) error {
	return pluginReleaseError(http.StatusForbidden, "PLUGIN_RELEASE_FORBIDDEN", err)
}
func pluginReleaseSessionNotFound(err error) error {
	return pluginReleaseError(http.StatusNotFound, "PLUGIN_RELEASE_SESSION_NOT_FOUND", err)
}
func pluginReleaseImportJobNotFound(err error) error {
	return pluginReleaseError(http.StatusNotFound, "PLUGIN_RELEASE_IMPORT_JOB_NOT_FOUND", err)
}
func pluginReleaseActiveSessionConflict(err error) error {
	return pluginReleaseError(http.StatusConflict, "PLUGIN_RELEASE_ACTIVE_SESSION_CONFLICT", err)
}
func pluginReleasePackageVerificationFailed(err error) error {
	return pluginReleaseError(http.StatusUnprocessableEntity, "PLUGIN_RELEASE_PACKAGE_VERIFICATION_FAILED", err)
}
func pluginReleaseUpstream(err error) error {
	return pluginReleaseError(http.StatusServiceUnavailable, "PLUGIN_RELEASE_UPSTREAM_DEPENDENCY", err)
}
