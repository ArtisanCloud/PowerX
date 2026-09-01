package iam

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	igwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
)

const (
	directoryMembersReadCapabilityID        = "com.corex.iam.members.read"
	directoryCatalogReadCapabilityID        = "com.corex.iam.directory.read"
	directoryAuthorizationCheckCapabilityID = "com.corex.iam.authorization.check"

	directoryMembersReadAPIKeyScope        = "_scope.iam.members.directory.read"
	directoryCatalogReadAPIKeyScope        = "_scope.iam.directory.catalog.read"
	directoryAuthorizationCheckAPIKeyScope = "_scope.iam.authorization.check"
)

const (
	DirectoryReasonInvalidArgument    = "IAM_INVALID_ARGUMENT"
	DirectoryReasonUnauthorized       = "IAM_UNAUTHORIZED"
	DirectoryReasonForbidden          = "IAM_FORBIDDEN"
	DirectoryReasonMemberNotFound     = "IAM_MEMBER_NOT_FOUND"
	DirectoryReasonSubjectMismatch    = "IAM_SUBJECT_MISMATCH"
	DirectoryReasonUpstreamDependency = "IAM_UPSTREAM_DEPENDENCY"
)

// DirectoryAccessService is the authorization boundary for service-actor IAM
// directory access. sts_direct controls routability; this service verifies the
// explicit capability grant carried by the tenant plugin credential.
type DirectoryAccessService struct {
	db *gorm.DB
}

type directoryServiceCredential struct {
	AllowedCapabilities []string `json:"allowed_capabilities,omitempty"`
}

func NewDirectoryAccessService(db *gorm.DB) *DirectoryAccessService {
	return &DirectoryAccessService{db: db}
}

func (s *DirectoryAccessService) AuthorizeMembersRead(ctx context.Context) (string, error) {
	return s.authorizeSTSCapability(ctx, directoryMembersReadCapabilityID)
}

func (s *DirectoryAccessService) AuthorizeDirectoryRead(ctx context.Context) (string, error) {
	return s.authorizeSTSCapability(ctx, directoryCatalogReadCapabilityID)
}

func (s *DirectoryAccessService) AuthorizeAuthorizationCheck(ctx context.Context) (string, error) {
	return s.authorizeSTSCapability(ctx, directoryAuthorizationCheckCapabilityID)
}

func (s *DirectoryAccessService) authorizeSTSCapability(ctx context.Context, capabilityID string) (string, error) {
	claims := reqctx.GetClaims(ctx)
	if claims == nil || !strings.EqualFold(strings.TrimSpace(claims.Issuer), "powerx-sts") || !directoryAudienceContains(claims.Audience, "powerx:api") {
		return "", directoryError(http.StatusUnauthorized, DirectoryReasonUnauthorized, errors.New("invalid sts service actor"))
	}
	tenantUUID, err := reqctx.RequireTenantUUID(ctx)
	if err != nil || strings.TrimSpace(tenantUUID) == "" {
		return "", directoryError(http.StatusUnauthorized, DirectoryReasonUnauthorized, errors.New("tenant context missing"))
	}
	tenantUUID, err = reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return "", directoryError(http.StatusUnauthorized, DirectoryReasonUnauthorized, err)
	}
	pluginID := strings.TrimSpace(claims.PluginID)
	if pluginID == "" {
		return "", directoryError(http.StatusUnauthorized, DirectoryReasonUnauthorized, errors.New("plugin identity missing"))
	}
	if err := s.requirePublishedDirectoryCapability(ctx, tenantUUID, capabilityID); err != nil {
		return "", err
	}
	return s.authorizeSTSCapabilityGrant(ctx, tenantUUID, pluginID, capabilityID)
}

// AuthorizeMembersReadAPIKey authorizes an established Gateway API-key service
// actor through its own key-specific permission grant. API keys are not STS
// tokens and must not be forced to carry STS claims.
func (s *DirectoryAccessService) AuthorizeMembersReadAPIKey(ctx context.Context, apiKeyHash string) (string, error) {
	return s.authorizeAPIKeyCapability(ctx, apiKeyHash, directoryMembersReadCapabilityID, directoryMembersReadAPIKeyScope, "directory-members")
}

func (s *DirectoryAccessService) AuthorizeDirectoryReadAPIKey(ctx context.Context, apiKeyHash string) (string, error) {
	return s.authorizeAPIKeyCapability(ctx, apiKeyHash, directoryCatalogReadCapabilityID, directoryCatalogReadAPIKeyScope, "directory-catalog")
}

func (s *DirectoryAccessService) AuthorizeAuthorizationCheckAPIKey(ctx context.Context, apiKeyHash string) (string, error) {
	return s.authorizeAPIKeyCapability(ctx, apiKeyHash, directoryAuthorizationCheckCapabilityID, directoryAuthorizationCheckAPIKeyScope, "authorization-check")
}

func (s *DirectoryAccessService) authorizeAPIKeyCapability(ctx context.Context, apiKeyHash, capabilityID, permissionScope, resource string) (string, error) {
	claims := reqctx.GetClaims(ctx)
	if claims == nil || !directoryStringContains(claims.Platforms, "api_key") {
		return "", directoryError(http.StatusUnauthorized, DirectoryReasonUnauthorized, errors.New("invalid api key service actor"))
	}
	tenantUUID, err := reqctx.RequireTenantUUID(ctx)
	if err != nil || strings.TrimSpace(tenantUUID) == "" {
		return "", directoryError(http.StatusUnauthorized, DirectoryReasonUnauthorized, errors.New("tenant context missing"))
	}
	tenantUUID, err = reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return "", directoryError(http.StatusUnauthorized, DirectoryReasonUnauthorized, err)
	}
	if strings.TrimSpace(apiKeyHash) == "" {
		return "", directoryError(http.StatusUnauthorized, DirectoryReasonUnauthorized, errors.New("api key identity missing"))
	}
	if err := s.requirePublishedDirectoryCapability(ctx, tenantUUID, capabilityID); err != nil {
		return "", err
	}
	key, err := igwrepo.NewIntegrationGatewayAPIKeyRepository(s.db).FindActiveByHash(ctx, tenantUUID, apiKeyHash)
	if err != nil {
		return "", DirectoryUpstreamDependencyError(err)
	}
	if key == nil {
		return "", directoryError(http.StatusUnauthorized, DirectoryReasonUnauthorized, errors.New("api key identity missing"))
	}
	allowed, err := igwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(s.db).HasPermission(
		ctx, key.UUID, permissionScope, "read", "api", resource,
	)
	if err != nil {
		return "", DirectoryUpstreamDependencyError(err)
	}
	if !allowed {
		return "", directoryError(http.StatusForbidden, DirectoryReasonForbidden, errors.New("api key capability grant missing"))
	}
	return tenantUUID, nil
}

func (s *DirectoryAccessService) requirePublishedDirectoryCapability(ctx context.Context, tenantUUID, capabilityID string) error {
	if s == nil || s.db == nil {
		return DirectoryUpstreamDependencyError(errors.New("directory authorization unavailable"))
	}
	var capability capmodels.CapabilityRecord
	if err := s.db.WithContext(ctx).Where("capability_id = ? AND status = ?", capabilityID, "published").First(&capability).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return DirectoryUpstreamDependencyError(errors.New("directory capability is not published"))
		}
		return DirectoryUpstreamDependencyError(err)
	}
	var registration capmodels.CapabilityRegistration
	if err := s.db.WithContext(ctx).Where("capability_id = ? AND tenant_uuid = ? AND status = ?", capabilityID, tenantUUID, "published").Order("version DESC").First(&registration).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return directoryError(http.StatusForbidden, DirectoryReasonForbidden, errors.New("tenant capability registration missing"))
		}
		return DirectoryUpstreamDependencyError(err)
	}
	return nil
}

func (s *DirectoryAccessService) authorizeSTSCapabilityGrant(ctx context.Context, tenantUUID, pluginID, capabilityID string) (string, error) {
	var credentialConfig dbsetting.PluginInstanceConfig
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", tenantUUID, pluginID, reposetting.KeyClientCredentials, true).First(&credentialConfig).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", directoryError(http.StatusForbidden, DirectoryReasonForbidden, errors.New("plugin service capability grant missing"))
		}
		return "", DirectoryUpstreamDependencyError(err)
	}
	var credential directoryServiceCredential
	if err := json.Unmarshal(credentialConfig.ValueJSON, &credential); err != nil {
		return "", DirectoryUpstreamDependencyError(err)
	}
	if !directoryStringContains(credential.AllowedCapabilities, capabilityID) {
		return "", directoryError(http.StatusForbidden, DirectoryReasonForbidden, errors.New("plugin service capability grant missing"))
	}
	return tenantUUID, nil
}

func DirectoryInvalidArgumentError(err error) error {
	return directoryError(http.StatusBadRequest, DirectoryReasonInvalidArgument, err)
}

func DirectoryMemberNotFoundError(err error) error {
	return directoryError(http.StatusNotFound, DirectoryReasonMemberNotFound, err)
}

func DirectorySubjectMismatchError(err error) error {
	return directoryError(http.StatusBadRequest, DirectoryReasonSubjectMismatch, err)
}

func DirectoryUpstreamDependencyError(err error) error {
	return directoryError(http.StatusServiceUnavailable, DirectoryReasonUpstreamDependency, err)
}

func directoryError(status int, reasonCode string, err error) error {
	return dto.NewErrorWithCode(status, reasonCode, reasonCode, err)
}

func directoryAudienceContains(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func directoryStringContains(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}
