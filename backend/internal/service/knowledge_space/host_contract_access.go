package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

const (
	KnowledgeDirectoryReadCapabilityID  = "com.corex.knowledge.directory.read"
	KnowledgeSearchReadCapabilityID     = "com.corex.knowledge.search.read"
	KnowledgeDocumentManageCapabilityID = "com.corex.knowledge.document.manage"
	KnowledgeCatalogReadCapabilityID    = "com.corex.knowledge.catalog.read"
	KnowledgeSpaceCreateCapabilityID    = "com.corex.knowledge.space.create"
	KnowledgeRetrievalReadCapabilityID  = "com.corex.knowledge.retrieval.read"

	knowledgeDirectoryAPIKeyScope = "_scope.knowledge.directory.read"
	knowledgeSearchAPIKeyScope    = "_scope.knowledge.search.read"
	knowledgeDocumentAPIKeyScope  = "_scope.knowledge.document.manage"
)

// HostContractAccess verifies both the published tenant registration and the
// credential-specific grant. sts_direct is intentionally not considered an
// authorization grant.
type HostContractAccess struct{ db *gorm.DB }

func NewHostContractAccess(db *gorm.DB) *HostContractAccess { return &HostContractAccess{db: db} }

func (s *HostContractAccess) AuthorizeDirectoryRead(ctx context.Context, apiKeyHash string) (string, error) {
	return s.authorize(ctx, apiKeyHash, KnowledgeDirectoryReadCapabilityID, knowledgeDirectoryAPIKeyScope, "directory", "read")
}

func (s *HostContractAccess) AuthorizeCatalogRead(ctx context.Context, apiKeyHash string) (string, error) {
	return s.authorize(ctx, apiKeyHash, KnowledgeCatalogReadCapabilityID, "_scope.knowledge.catalog.read", "catalog", "read")
}
func (s *HostContractAccess) AuthorizeSpaceCreate(ctx context.Context, apiKeyHash string) (string, error) {
	return s.authorize(ctx, apiKeyHash, KnowledgeSpaceCreateCapabilityID, "_scope.knowledge.space.create", "space", "create")
}
func (s *HostContractAccess) AuthorizeSearchRead(ctx context.Context, apiKeyHash string) (string, error) {
	return s.authorize(ctx, apiKeyHash, KnowledgeSearchReadCapabilityID, knowledgeSearchAPIKeyScope, "search", "read")
}

func (s *HostContractAccess) AuthorizeRetrievalRead(ctx context.Context, apiKeyHash string) (string, error) {
	return s.authorize(ctx, apiKeyHash, KnowledgeRetrievalReadCapabilityID, "_scope.knowledge.retrieval.read", "retrieval", "read")
}
func (s *HostContractAccess) AuthorizeDocumentManage(ctx context.Context, apiKeyHash string) (string, error) {
	return s.authorize(ctx, apiKeyHash, KnowledgeDocumentManageCapabilityID, knowledgeDocumentAPIKeyScope, "document", "manage")
}

type knowledgeServiceCredential struct {
	AllowedCapabilities []string `json:"allowed_capabilities,omitempty"`
}

func (s *HostContractAccess) authorize(ctx context.Context, apiKeyHash, capabilityID, apiKeyScope, resource, action string) (string, error) {
	if s == nil || s.db == nil {
		return "", KnowledgeUpstreamDependencyError(errors.New("knowledge authorization unavailable"))
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil {
		return "", KnowledgeUnauthorizedError(errors.New("service actor missing"))
	}
	tenantUUID, err := reqctx.RequireTenantUUID(ctx)
	if err != nil {
		return "", KnowledgeUnauthorizedError(err)
	}
	tenantUUID, err = reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return "", KnowledgeUnauthorizedError(err)
	}
	if err := s.requirePublished(ctx, tenantUUID, capabilityID); err != nil {
		return "", err
	}
	if knowledgeContains(claims.Platforms, "api_key") {
		return s.authorizeAPIKey(ctx, tenantUUID, apiKeyHash, apiKeyScope, resource, action)
	}
	if !strings.EqualFold(strings.TrimSpace(claims.Issuer), "powerx-sts") || !knowledgeContains(claims.Audience, "powerx:api") {
		return "", KnowledgeUnauthorizedError(errors.New("invalid sts service actor"))
	}
	pluginID := strings.TrimSpace(claims.PluginID)
	if pluginID == "" {
		return "", KnowledgeUnauthorizedError(errors.New("plugin identity missing"))
	}
	var credential dbsetting.PluginInstanceConfig
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", tenantUUID, pluginID, settingrepo.KeyClientCredentials, true).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", KnowledgeForbiddenError(errors.New("plugin capability grant missing"))
		}
		return "", KnowledgeUpstreamDependencyError(err)
	}
	var value knowledgeServiceCredential
	if err := json.Unmarshal(credential.ValueJSON, &value); err != nil {
		return "", KnowledgeUpstreamDependencyError(err)
	}
	if !knowledgeContains(value.AllowedCapabilities, capabilityID) {
		return "", KnowledgeForbiddenError(errors.New("plugin capability grant missing"))
	}
	return tenantUUID, nil
}

func (s *HostContractAccess) requirePublished(ctx context.Context, tenantUUID, capabilityID string) error {
	var capability capmodels.CapabilityRecord
	if err := s.db.WithContext(ctx).Where("capability_id = ? AND status = ?", capabilityID, "published").First(&capability).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return KnowledgeUpstreamDependencyError(errors.New("knowledge capability is not published"))
		}
		return KnowledgeUpstreamDependencyError(err)
	}
	var registration capmodels.CapabilityRegistration
	if err := s.db.WithContext(ctx).Where("capability_id = ? AND tenant_uuid = ? AND status = ?", capabilityID, tenantUUID, "published").Order("version DESC").First(&registration).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return KnowledgeForbiddenError(errors.New("tenant capability registration missing"))
		}
		return KnowledgeUpstreamDependencyError(err)
	}
	return nil
}

func (s *HostContractAccess) authorizeAPIKey(ctx context.Context, tenantUUID, apiKeyHash, scope, resource, action string) (string, error) {
	if strings.TrimSpace(apiKeyHash) == "" {
		return "", KnowledgeUnauthorizedError(errors.New("api key identity missing"))
	}
	key, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(s.db).FindActiveByHash(ctx, tenantUUID, apiKeyHash)
	if err != nil {
		return "", KnowledgeUpstreamDependencyError(err)
	}
	if key == nil {
		return "", KnowledgeUnauthorizedError(errors.New("api key identity missing"))
	}
	allowed, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(s.db).HasPermission(ctx, key.UUID, scope, action, "api", resource)
	if err != nil {
		return "", KnowledgeUpstreamDependencyError(err)
	}
	if !allowed {
		return "", KnowledgeForbiddenError(errors.New("api key capability grant missing"))
	}
	return tenantUUID, nil
}

func knowledgeContains(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}
