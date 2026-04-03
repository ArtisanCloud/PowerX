package seed

import (
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repoai "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SeedDefaultAIConfig seeds global AI defaults into ai_provider_credentials/ai_model_profiles
// scoped by env (POWERX_ENV, fallback: dev).
func SeedDefaultAIConfig(db *gorm.DB, cfg *config.Config) error {
	if db == nil || cfg == nil {
		return nil
	}
	env := strings.TrimSpace(envOrDefault("POWERX_ENV", "dev"))
	if env == "" {
		env = "dev"
	}

	llm := cfg.AI.Defaults.LLM
	provider := strings.TrimSpace(llm.Provider)
	model := strings.TrimSpace(llm.Model)
	if provider == "" || model == "" {
		return nil
	}

	cred := &dbmodel.AIProviderCredential{
		Name:     utils.Slug(env + "-" + provider),
		Provider: provider,
		Data: datatypes.JSONMap{
			"base_url":         strings.TrimSpace(llm.Endpoint),
			"region":           strings.TrimSpace(llm.Region),
			"organization":     strings.TrimSpace(llm.Organization),
			"azure_deployment": strings.TrimSpace(llm.AzureDeployment),
			"seeded_by":        "cmd/database seed",
			"seeded_modality":  "llm",
		},
	}
	prof := &dbmodel.AIModelProfile{
		Modality: "llm",
		Provider: provider,
		Model:    model,
		Defaults: datatypes.JSONMap{
			"temperature": llm.Temperature,
			"topP":        llm.TopP,
			"maxTokens":   llm.MaxTokens,
			"stream":      llm.Stream,
		},
		Tags: []string{"llm", "seed", "default"},
	}
	ctx := seedCtx()
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repoai.NewAIProviderCredentialRepository(tx).
			UpsertByScopeNameProvider(ctx, env, nil, cred); err != nil {
			return fmt.Errorf("upsert ai credential failed: %w", err)
		}
		if err := repoai.NewAIModelProfileRepository(tx).
			UpsertByScopeModalityProviderModel(ctx, env, nil, prof); err != nil {
			return fmt.Errorf("upsert ai profile failed: %w", err)
		}
		if err := repoai.NewAIRoutePolicyRepository(tx).
			UpsertDefaultByScopeModality(ctx, env, nil, "llm", provider, model); err != nil {
			return fmt.Errorf("upsert ai route policy failed: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("seed default ai config failed (env=%s provider=%s model=%s): %w", env, provider, model, err)
	}
	return nil
}
