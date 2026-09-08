package seed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	apikeypermissions "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeligw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	dbmtenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	repoigw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

type devSeedAPIKey struct {
	EnvName string
	Name    string
	Key     string
}

var defaultDevAPIKeys = []devSeedAPIKey{
	{
		EnvName: "POWERX_PLUGIN_API_KEY",
		Name:    "PowerX Plugin Development Key",
		Key:     "pxk_Gz0XJ8jjMCxFIKjwDfu1svJaKP3x1Ww0p7dE1HwUBKwzRS7n",
	},
	{
		EnvName: "MEDIA_STUDIO_API_KEY",
		Name:    "Media Studio Development Key",
		Key:     "pxk_wOUQfEVDLKb1hyopN5rrHajk6Fa3cfGkCBPuetFUkLa0zaI4",
	},
	{
		EnvName: "ECOMMERCE_API_KEY",
		Name:    "Ecommerce Development Key",
		Key:     "pxk_fFXh3rXOAQv7ZBamjL7OkvF7qLwDETgOjBr1ofsA9wOXSfr2",
	},
	{
		EnvName: "SCRM_API_KEY",
		Name:    "SCRM Development Key",
		Key:     "pxk_mIuVIJp3aUxrGghYNMonfUqpIfO2KaICPdDzkB7ZYe1HAcI6",
	},
}

func SeedDefaultDevAPIKeys(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	ctx := seedCtx()

	ten, err := tenantrepo.NewTenantRepository(db).EnsureByKey(ctx, "system", "System", dbmtenant.TenantPlanFree, dbmtenant.TenantTypeSystem)
	if err != nil {
		return fmt.Errorf("ensure system tenant failed: %w", err)
	}
	tenantUUID := strings.TrimSpace(ten.UUID.String())
	if tenantUUID == "" {
		return fmt.Errorf("system tenant uuid is empty")
	}

	profile, permissionRows, err := apikeypermissions.EnsureTenantDefaultProfile(ctx, db, tenantUUID, nil)
	if err != nil {
		return fmt.Errorf("ensure default api key profile failed: %w", err)
	}
	if profile == nil || profile.ID == 0 {
		return fmt.Errorf("default api key profile not found")
	}

	permissions, err := resolveProfilePermissions(ctx, db, permissionRows)
	if err != nil {
		return err
	}
	knowledgeHostPermissions, err := resolveKnowledgeHostDevPermissions(ctx, db)
	if err != nil {
		return err
	}
	metadataHostPermissions, err := resolveMetadataHostDevPermissions(ctx, db)
	if err != nil {
		return err
	}

	for _, item := range defaultDevAPIKeys {
		keyPermissions := permissions
		if item.EnvName == "POWERX_PLUGIN_API_KEY" {
			keyPermissions = appendUniqueAPIKeyPermissions(permissions, knowledgeHostPermissions)
			keyPermissions = appendUniqueAPIKeyPermissions(keyPermissions, metadataHostPermissions)
		}
		if err := upsertDevAPIKey(ctx, db, tenantUUID, profile.ID, item, keyPermissions); err != nil {
			return err
		}
	}

	return nil
}

func resolveMetadataHostDevPermissions(ctx context.Context, db *gorm.DB) ([]modeligw.IntegrationGatewayAPIKeyPermission, error) {
	_ = ctx
	_ = db
	resources := []string{"dictionary", "taxonomy", "tag", "resource_type"}
	out := make([]modeligw.IntegrationGatewayAPIKeyPermission, 0, len(resources)*2)
	for _, resource := range resources {
		for _, action := range []string{"read", "manage"} {
			out = append(out, modeligw.IntegrationGatewayAPIKeyPermission{Scope: "_scope.metadata." + resource + "." + action, Action: action, ResourceType: "api", ResourcePattern: resource, Effect: "allow"})
		}
	}
	return out, nil
}

// resolveKnowledgeHostDevPermissions resolves the explicit Knowledge Host
// scopes for the dedicated PowerX plugin development key. Production keys
// remain profile-driven and must be explicitly granted by an administrator.
func resolveKnowledgeHostDevPermissions(ctx context.Context, db *gorm.DB) ([]modeligw.IntegrationGatewayAPIKeyPermission, error) {
	const module = "knowledge_space"
	wantedScopes := map[string]struct{}{
		"_scope.knowledge.directory.read":  {},
		"_scope.knowledge.search.read":     {},
		"_scope.knowledge.document.manage": {},
	}
	var rows []modeliam.Permission
	if err := db.WithContext(ctx).Where("module = ? AND allow_api_key = ? AND status = ?", module, true, modeliam.PermissionStatusActive).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load knowledge host api key permissions: %w", err)
	}
	out := make([]modeligw.IntegrationGatewayAPIKeyPermission, 0, len(rows))
	for _, row := range rows {
		resolved, ok := apikeypermissions.ResolvePermission(row)
		if !ok {
			continue
		}
		if _, ok := wantedScopes[resolved.Scope]; !ok {
			continue
		}
		out = append(out, modeligw.IntegrationGatewayAPIKeyPermission{Scope: resolved.Scope, Action: resolved.Action, ResourceType: resolved.ResourceType, ResourcePattern: resolved.ResourcePattern, PluginID: resolved.PluginID, Effect: resolved.Effect})
	}
	return out, nil
}

func appendUniqueAPIKeyPermissions(base, additions []modeligw.IntegrationGatewayAPIKeyPermission) []modeligw.IntegrationGatewayAPIKeyPermission {
	seen := make(map[string]struct{}, len(base)+len(additions))
	out := make([]modeligw.IntegrationGatewayAPIKeyPermission, 0, len(base)+len(additions))
	for _, item := range append(base, additions...) {
		key := strings.Join([]string{item.Scope, item.Action, item.ResourceType, item.ResourcePattern, item.PluginID, item.Effect}, "\x00")
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func resolveProfilePermissions(ctx context.Context, db *gorm.DB, permissionIDs []uint64) ([]modeligw.IntegrationGatewayAPIKeyPermission, error) {
	rows, err := infraiam.NewPermissionRepository(db).FindByIDs(ctx, permissionIDs)
	if err != nil {
		return nil, fmt.Errorf("load api key permissions failed: %w", err)
	}

	out := make([]modeligw.IntegrationGatewayAPIKeyPermission, 0, len(rows))
	for _, permission := range rows {
		if permission == nil || permission.Status != modeliam.PermissionStatusActive || !permission.AllowAPIKey {
			continue
		}
		resolved, ok := apikeypermissions.ResolvePermission(*permission)
		if !ok {
			continue
		}
		out = append(out, modeligw.IntegrationGatewayAPIKeyPermission{
			Scope:           resolved.Scope,
			Action:          resolved.Action,
			ResourceType:    resolved.ResourceType,
			ResourcePattern: resolved.ResourcePattern,
			PluginID:        resolved.PluginID,
			Effect:          resolved.Effect,
		})
	}
	return out, nil
}

func upsertDevAPIKey(
	ctx context.Context,
	db *gorm.DB,
	tenantUUID string,
	profileID uint64,
	item devSeedAPIKey,
	basePermissions []modeligw.IntegrationGatewayAPIKeyPermission,
) error {
	plain := strings.TrimSpace(item.Key)
	if plain == "" {
		return nil
	}
	if !strings.HasPrefix(plain, "pxk_") {
		return fmt.Errorf("%s must start with pxk_", item.EnvName)
	}
	hash := hashAPIKey(plain)
	prefix := keyPrefix(plain)
	now := time.Now().UnixMilli()

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		iamRepo := infraiam.NewAPIKeyRepository(tx)
		profileRepo := infraiam.NewAPIKeyProfileRepository(tx)
		keyRepo := repoigw.NewIntegrationGatewayAPIKeyRepository(tx)
		keyPermRepo := repoigw.NewIntegrationGatewayAPIKeyPermissionRepository(tx)

		profile, err := profileRepo.FindByKey(ctx, tenantUUID, apikeypermissions.DefaultAPIKeyProfileKey)
		if err != nil {
			return fmt.Errorf("load default api key profile failed: %w", err)
		}
		if profile == nil || profile.ID == 0 {
			return fmt.Errorf("default api key profile missing for tenant %s", tenantUUID)
		}

		var iamKey modeliam.APIKey
		if err := tx.WithContext(ctx).Where("key_hash = ?", hash).First(&iamKey).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := iamRepo.Create(ctx, &modeliam.APIKey{
				TenantUUID:  tenantUUID,
				ProfileID:   profileID,
				KeyHash:     hash,
				CreatedAtMs: now,
			}); err != nil {
				return fmt.Errorf("create iam api key failed: %w", err)
			}
		} else {
			if !strings.EqualFold(strings.TrimSpace(iamKey.TenantUUID), tenantUUID) {
				return fmt.Errorf("%s already exists under another tenant", item.EnvName)
			}
			if err := tx.WithContext(ctx).Model(&modeliam.APIKey{}).
				Where("id = ?", iamKey.ID).
				Updates(map[string]interface{}{
					"tenant_uuid":   tenantUUID,
					"profile_id":    profileID,
					"revoked_at_ms": nil,
				}).Error; err != nil {
				return fmt.Errorf("update iam api key failed: %w", err)
			}
		}

		var gatewayKey modeligw.IntegrationGatewayAPIKey
		if err := tx.WithContext(ctx).
			Where("tenant_uuid = ? AND key_hash = ?", tenantUUID, hash).
			First(&gatewayKey).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			created, createErr := keyRepo.Create(ctx, &modeligw.IntegrationGatewayAPIKey{
				TenantUUID:  tenantUUID,
				ProfileID:   profileID,
				Name:        item.Name,
				Description: "seeded development api key",
				KeyPrefix:   prefix,
				KeyHash:     hash,
				Status:      "active",
				CreatedBy:   "seed",
				UpdatedBy:   "seed",
			})
			if createErr != nil {
				return fmt.Errorf("create integration api key failed: %w", createErr)
			}
			gatewayKey = *created
		} else {
			if err := tx.WithContext(ctx).Model(&modeligw.IntegrationGatewayAPIKey{}).
				Where("uuid = ?", gatewayKey.UUID).
				Updates(map[string]interface{}{
					"profile_id":   profileID,
					"name":         item.Name,
					"description":  "seeded development api key",
					"key_prefix":   prefix,
					"status":       "active",
					"updated_by":   "seed",
					"last_used_at": nil,
				}).Error; err != nil {
				return fmt.Errorf("update integration api key failed: %w", err)
			}
		}

		permissions := make([]modeligw.IntegrationGatewayAPIKeyPermission, 0, len(basePermissions))
		for _, permission := range basePermissions {
			permissions = append(permissions, modeligw.IntegrationGatewayAPIKeyPermission{
				APIKeyUUID:      gatewayKey.UUID,
				Scope:           permission.Scope,
				Action:          permission.Action,
				ResourceType:    permission.ResourceType,
				ResourcePattern: permission.ResourcePattern,
				PluginID:        permission.PluginID,
				Effect:          permission.Effect,
			})
		}
		if err := keyPermRepo.ReplaceAll(ctx, gatewayKey.UUID, permissions); err != nil {
			return fmt.Errorf("replace api key permissions failed: %w", err)
		}

		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] ensured dev api key %s prefix=%s tenant=%s", item.EnvName, prefix, tenantUUID)
		return nil
	})
}

func hashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:])
}

func keyPrefix(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
