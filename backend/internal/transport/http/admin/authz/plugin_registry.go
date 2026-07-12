package authz

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	apikeycache "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeycache"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	integrationrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

const (
	ScopePluginSkillRegistrySync     = "_scope.plugin.skill_registry.sync"
	ScopePluginAgentRegistrySync     = "_scope.plugin.agent_registry.sync"
	ScopePluginCapabilityCatalogSync = "_scope.plugin.capability_catalog.sync"
	ScopePluginDebugHostRegister     = "_scope.plugin.debug_host.register"
)

// PluginRegistrySyncMiddleware allows root callers and API keys with the required plugin registry sync scope.
func PluginRegistrySyncMiddleware(deps *shared.Deps, requiredScope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if reqctx.IsRoot(c.Request.Context()) {
			c.Next()
			return
		}
		if !IsAPIKeyAuth(c) {
			dto.ResponseError(c, http.StatusForbidden, "forbidden: admin root only", nil)
			c.Abort()
			return
		}
		if err := AuthorizeAPIKeyPluginRegistrySync(c, deps, requiredScope); err != nil {
			dto.ResponseError(c, http.StatusForbidden, "api key forbidden: insufficient plugin registry permissions", err)
			c.Abort()
			return
		}
		c.Next()
	}
}

// AdminOrPluginRegistrySyncMiddleware allows normal admin JWT callers, while API key callers must carry the required plugin registry sync scope.
func AdminOrPluginRegistrySyncMiddleware(deps *shared.Deps, requiredScope string) gin.HandlerFunc {
	adminOnly := middleware.AdminOnlyMiddleware()
	return func(c *gin.Context) {
		if reqctx.IsRoot(c.Request.Context()) {
			c.Next()
			return
		}
		if IsAPIKeyAuth(c) {
			if err := AuthorizeAPIKeyPluginRegistrySync(c, deps, requiredScope); err != nil {
				dto.ResponseError(c, http.StatusForbidden, "api key forbidden: insufficient plugin registry permissions", err)
				c.Abort()
				return
			}
			c.Next()
			return
		}
		adminOnly(c)
	}
}

func AuthorizeAPIKeyPluginRegistrySync(c *gin.Context, deps *shared.Deps, requiredScope string) error {
	if deps == nil || deps.DB == nil {
		return fmt.Errorf("api key permission store unavailable")
	}
	requiredScope = strings.TrimSpace(requiredScope)
	if requiredScope == "" {
		return fmt.Errorf("api key permission scope is required")
	}
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	keyHash := APIKeyHashFromContext(c)
	if keyHash == "" {
		return fmt.Errorf("api key hash missing")
	}
	resource := strings.ToUpper(strings.TrimSpace(c.Request.Method)) + ":" + CanonicalAPIPath(c.Request.URL.Path)
	if allowedCached, hit, cacheErr := apikeycache.GetPermissionDecision(
		c.Request.Context(),
		keyHash,
		"sync",
		"api",
		resource,
	); cacheErr == nil && hit {
		if allowedCached {
			return nil
		}
		return fmt.Errorf("plugin registry permission denied: %s", resource)
	}

	keyRepo := integrationrepo.NewIntegrationGatewayAPIKeyRepository(deps.DB)
	keyModel, err := keyRepo.FindActiveByHash(c.Request.Context(), tenantUUID, keyHash)
	if err != nil {
		return err
	}
	if keyModel == nil {
		return fmt.Errorf("api key not found")
	}
	permRepo := integrationrepo.NewIntegrationGatewayAPIKeyPermissionRepository(deps.DB)
	ok, err := permRepo.HasPermission(
		c.Request.Context(),
		keyModel.UUID,
		requiredScope,
		"sync",
		"api",
		resource,
	)
	if err != nil {
		return err
	}
	_ = apikeycache.SetPermissionDecision(c.Request.Context(), keyHash, "sync", "api", resource, ok)
	if !ok {
		return fmt.Errorf("plugin registry permission denied: %s", resource)
	}
	return nil
}

func IsAPIKeyAuth(c *gin.Context) bool {
	raw, ok := c.Get("auth_source")
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(raw)), "api_key")
}

func APIKeyHashFromContext(c *gin.Context) string {
	raw, ok := c.Get("auth_api_key_hash")
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func CanonicalAPIPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/api/v1"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	return path
}
