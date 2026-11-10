package agent

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestVisionTrialCurrentFormDoesNotRequireStoredModelOrConnection(t *testing.T) {
	s, reg := onlineCatalogFixture(t)
	ctx, tenant := context.Background(), onlineTestTenant
	localBefore, _ := reg.Models("vlm", "openai")
	form := VisionTrialConnection{CurrentForm: true, APIKey: "new-private-fixture", BaseURL: "https://temporary.example.invalid/prefix/v1", Organization: "draft-org"}
	model, config, err := s.PrepareVisionTrialConfig(ctx, "dev", tenant, "openai", "unsaved-model", form, map[string]any{"max_tokens": 3072})
	require.NoError(t, err)
	require.Equal(t, "unsaved-model", model.Model.ID)
	require.Equal(t, "openai", model.Model.Operations[catalog.VisionUnderstanding].Driver)
	require.Equal(t, form.BaseURL, config.Endpoint)
	require.Equal(t, form.APIKey, config.APIKey)
	require.Equal(t, "draft-org", config.Organization)
	localAfter, _ := reg.Models("vlm", "openai")
	require.Equal(t, localBefore, localAfter)
	for _, table := range []string{"ai_model_profiles", "ai_provider_credentials", "cfg_tenant_settings", "iam_tenant_key_pairs"} {
		var count int64
		require.NoError(t, s.db.Table(table).Count(&count).Error)
		require.Zero(t, count, table)
	}
}

func TestVisionTrialReusesOnlySavedKeyAndRespectsExplicitDefaultAddress(t *testing.T) {
	s, _ := onlineCatalogFixture(t)
	ctx, tenant := context.Background(), onlineTestTenant
	data, err := s.tks.SealSensitive(ctx, "dev", tenant, datatypes.JSONMap{"api_key": "saved-private-fixture", "base_url": "https://old.example.invalid/v1", "organization": "old-org", "azure_deployment": "old-deployment"}, "api_key")
	require.NoError(t, err)
	require.NoError(t, s.db.Create(&am.AIProviderCredential{Env: "dev", TenantUUID: &tenant, Name: "dev-openai", Provider: "openai", Data: data}).Error)
	var before am.AIProviderCredential
	require.NoError(t, s.db.First(&before).Error)
	for _, key := range []string{"", "new-private-fixture"} {
		_, config, err := s.PrepareVisionTrialConfig(ctx, "dev", tenant, "openai", "unsaved-model", VisionTrialConnection{CurrentForm: true, APIKey: key}, nil)
		require.NoError(t, err)
		require.Equal(t, "https://api.openai.com/v1", config.Endpoint)
		require.Empty(t, config.Organization)
		require.Empty(t, config.AzureDeployment)
		if key == "" {
			require.Equal(t, "saved-private-fixture", config.APIKey)
		} else {
			require.Equal(t, key, config.APIKey)
		}
	}
	var after am.AIProviderCredential
	require.NoError(t, s.db.First(&after).Error)
	require.Equal(t, before, after)
	for _, test := range []struct{ env, tenant string }{{"prod", tenant}, {"dev", "22222222-2222-4222-8222-222222222222"}} {
		_, _, err := s.PrepareVisionTrialConfig(ctx, test.env, test.tenant, "openai", "unsaved-model", VisionTrialConnection{CurrentForm: true}, nil)
		require.Equal(t, "AI_CONNECTION_CONFIG_INVALID", dto.CodeOf(err))
	}
	_, _, err = s.PrepareVisionTrialConfig(ctx, "dev", tenant, "openai", "tts-1", VisionTrialConnection{CurrentForm: true, APIKey: "fixture"}, nil)
	require.Equal(t, "AI_VISION_IMAGE_INPUT_UNSUPPORTED", dto.CodeOf(err))
}
