package media

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
	MediaAssetsReadCapabilityID       = "com.corex.media.assets.read"
	MediaAssetsManageCapabilityID     = "com.corex.media.assets.manage"
	MediaAssetsTransferCapabilityID   = "com.corex.media.assets.transfer"
	MediaAssetsVariantsCapabilityID   = "com.corex.media.assets.variants.manage"
	mediaAssetsReadAPIKeyScope        = "_scope.media.assets.read"
	mediaAssetsManageAPIKeyScope      = "_scope.media.assets.manage"
	mediaAssetsTransferAPIKeyScope    = "_scope.media.assets.transfer"
	mediaAssetsVariantsAPIKeyScope    = "_scope.media.assets.variants.manage"
	MediaReasonInvalidArgument        = "MEDIA_INVALID_ARGUMENT"
	MediaReasonUnauthorized           = "MEDIA_UNAUTHORIZED"
	MediaReasonForbidden              = "MEDIA_FORBIDDEN"
	MediaReasonAssetNotFound          = "MEDIA_ASSET_NOT_FOUND"
	MediaReasonAssetStateInvalid      = "MEDIA_ASSET_STATE_INVALID"
	MediaReasonUploadValidationFailed = "MEDIA_UPLOAD_VALIDATION_FAILED"
	MediaReasonUpstreamDependency     = "MEDIA_UPSTREAM_DEPENDENCY"
)

// HostContractAccess enforces publication, tenant registration, and the actual
// STS/API-key grant. sts_direct only controls route admission and never grants access.
type HostContractAccess struct{ db *gorm.DB }

func NewHostContractAccess(db *gorm.DB) *HostContractAccess { return &HostContractAccess{db: db} }

func (s *HostContractAccess) AuthorizeRead(ctx context.Context, apiKeyHash string) (string, error) {
	return s.authorize(ctx, apiKeyHash, MediaAssetsReadCapabilityID, mediaAssetsReadAPIKeyScope, "assets", "read")
}
func (s *HostContractAccess) AuthorizeManage(ctx context.Context, apiKeyHash string) (string, error) {
	return s.authorize(ctx, apiKeyHash, MediaAssetsManageCapabilityID, mediaAssetsManageAPIKeyScope, "assets", "manage")
}
func (s *HostContractAccess) AuthorizeTransfer(ctx context.Context, apiKeyHash string) (string, error) {
	return s.authorize(ctx, apiKeyHash, MediaAssetsTransferCapabilityID, mediaAssetsTransferAPIKeyScope, "assets", "transfer")
}
func (s *HostContractAccess) AuthorizeVariantsManage(ctx context.Context, apiKeyHash string) (string, error) {
	return s.authorize(ctx, apiKeyHash, MediaAssetsVariantsCapabilityID, mediaAssetsVariantsAPIKeyScope, "variants", "manage")
}

type mediaServiceCredential struct {
	AllowedCapabilities []string `json:"allowed_capabilities,omitempty"`
}

func (s *HostContractAccess) authorize(ctx context.Context, apiKeyHash, capabilityID, apiKeyScope, resource, action string) (string, error) {
	if s == nil || s.db == nil {
		return "", MediaUpstreamDependencyError(errors.New("media authorization unavailable"))
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil {
		return "", MediaUnauthorizedError(errors.New("service actor missing"))
	}
	tenantUUID, err := reqctx.RequireTenantUUID(ctx)
	if err != nil {
		return "", MediaUnauthorizedError(err)
	}
	tenantUUID, err = reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return "", MediaUnauthorizedError(err)
	}
	if err = s.requirePublished(ctx, tenantUUID, capabilityID); err != nil {
		return "", err
	}
	if mediaContains(claims.Platforms, "api_key") {
		return s.authorizeAPIKey(ctx, tenantUUID, apiKeyHash, apiKeyScope, resource, action)
	}
	if !strings.EqualFold(strings.TrimSpace(claims.Issuer), "powerx-sts") || !mediaContains(claims.Audience, "powerx:api") {
		return "", MediaUnauthorizedError(errors.New("invalid sts service actor"))
	}
	pluginID := strings.TrimSpace(claims.PluginID)
	if pluginID == "" {
		return "", MediaUnauthorizedError(errors.New("plugin identity missing"))
	}
	var credential dbsetting.PluginInstanceConfig
	if err = s.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", tenantUUID, pluginID, settingrepo.KeyClientCredentials, true).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", MediaForbiddenError(errors.New("plugin capability grant missing"))
		}
		return "", MediaUpstreamDependencyError(err)
	}
	var value mediaServiceCredential
	if err = json.Unmarshal(credential.ValueJSON, &value); err != nil {
		return "", MediaUpstreamDependencyError(err)
	}
	if !mediaContains(value.AllowedCapabilities, capabilityID) {
		return "", MediaForbiddenError(errors.New("plugin capability grant missing"))
	}
	return tenantUUID, nil
}

func (s *HostContractAccess) requirePublished(ctx context.Context, tenantUUID, capabilityID string) error {
	var capability capmodels.CapabilityRecord
	if err := s.db.WithContext(ctx).Where("capability_id = ? AND status = ?", capabilityID, "published").First(&capability).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return MediaForbiddenError(errors.New("media capability is not published"))
		}
		return MediaUpstreamDependencyError(err)
	}
	var registration capmodels.CapabilityRegistration
	if err := s.db.WithContext(ctx).Where("capability_id = ? AND tenant_uuid = ?", capabilityID, tenantUUID).Order("version DESC").First(&registration).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return MediaForbiddenError(errors.New("tenant capability registration missing"))
		}
		return MediaUpstreamDependencyError(err)
	}
	if registration.Status != "published" {
		return MediaForbiddenError(errors.New("media.registration_inactive"))
	}
	return nil
}

func (s *HostContractAccess) authorizeAPIKey(ctx context.Context, tenantUUID, apiKeyHash, scope, resource, action string) (string, error) {
	if strings.TrimSpace(apiKeyHash) == "" {
		return "", MediaUnauthorizedError(errors.New("api key identity missing"))
	}
	key, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(s.db).FindActiveByHash(ctx, tenantUUID, apiKeyHash)
	if err != nil {
		return "", MediaUpstreamDependencyError(err)
	}
	if key == nil {
		return "", MediaUnauthorizedError(errors.New("api key identity missing"))
	}
	allowed, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(s.db).HasPermission(ctx, key.UUID, scope, action, "api", resource)
	if err != nil {
		return "", MediaUpstreamDependencyError(err)
	}
	if !allowed {
		return "", MediaForbiddenError(errors.New("api key capability grant missing"))
	}
	return tenantUUID, nil
}

func mediaContains(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}
func mediaError(status int, code string, err error) error {
	return dto.NewErrorWithCode(status, code, code, err)
}
func MediaInvalidArgumentError(err error) error {
	return mediaError(http.StatusBadRequest, MediaReasonInvalidArgument, err)
}
func MediaUnauthorizedError(err error) error {
	return mediaError(http.StatusUnauthorized, MediaReasonUnauthorized, err)
}
func MediaForbiddenError(err error) error {
	return mediaError(http.StatusForbidden, MediaReasonForbidden, err)
}
func MediaAssetNotFoundError(err error) error {
	return mediaError(http.StatusNotFound, MediaReasonAssetNotFound, err)
}
func MediaAssetStateInvalidError(err error) error {
	return mediaError(http.StatusConflict, MediaReasonAssetStateInvalid, err)
}
func MediaUploadValidationFailedError(err error) error {
	return mediaError(http.StatusUnprocessableEntity, MediaReasonUploadValidationFailed, err)
}
func MediaUpstreamDependencyError(err error) error {
	return mediaError(http.StatusServiceUnavailable, MediaReasonUpstreamDependency, err)
}
