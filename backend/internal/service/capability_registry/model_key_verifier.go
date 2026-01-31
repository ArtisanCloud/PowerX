package capability_registry

import (
	"context"
	"fmt"
	"strings"

	repoai "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"gorm.io/gorm"
)

// TenantModelKeyVerifier validates model_key ownership for a tenant.
type TenantModelKeyVerifier struct {
	profiles *repoai.AIModelProfileRepository
}

// NewTenantModelKeyVerifier constructs a tenant-scoped model_key verifier.
func NewTenantModelKeyVerifier(db *gorm.DB) *TenantModelKeyVerifier {
	if db == nil {
		return nil
	}
	return &TenantModelKeyVerifier{
		profiles: repoai.NewAIModelProfileRepository(db),
	}
}

// VerifyModelKey checks if model_key belongs to tenant configuration.
func (v *TenantModelKeyVerifier) VerifyModelKey(ctx context.Context, tenantUUID, env, modality, modelKey string) error {
	if v == nil || v.profiles == nil {
		return nil
	}
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return fmt.Errorf("tenant_uuid required for model verification")
	}
	modality = strings.TrimSpace(modality)
	if modality == "" {
		return fmt.Errorf("modality required for model verification")
	}
	modelKey = strings.TrimSpace(modelKey)
	if modelKey == "" {
		return fmt.Errorf("model_key required")
	}
	provider, model := splitModelKey(modelKey)
	if provider == "" || model == "" {
		return fmt.Errorf("invalid model_key %s", modelKey)
	}
	env = strings.TrimSpace(env)
	if env == "" {
		env = "default"
	}
	_, err := v.profiles.FindByScopeModalityProviderModel(ctx, env, &tenantUUID, modality, provider, model)
	if err != nil {
		return fmt.Errorf("model_key %s not configured for tenant", modelKey)
	}
	return nil
}

func splitModelKey(modelKey string) (string, string) {
	if strings.Contains(modelKey, "/") {
		parts := strings.SplitN(modelKey, "/", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if strings.Contains(modelKey, ":") {
		parts := strings.SplitN(modelKey, ":", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", ""
}
