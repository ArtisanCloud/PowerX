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

func TestImageDefaultsUseContractAndRejectInvalidValuesBeforeWriting(t *testing.T) {
	s := environmentAliasFixture(t)
	tenant, ctx := onlineTestTenant, context.Background()
	savedOnlineCredential(t, s, tenant, "dev", "openai", "https://provider.example.invalid/v1")
	var before am.AIProviderCredential
	require.NoError(t, s.db.First(&before).Error)
	for _, model := range []string{"gpt-image-2", "gpt-image-2-2026-04-21", "chatgpt-image-latest"} {
		profile := &am.AIModelProfile{Modality: "image", Provider: "openai", Model: model, Defaults: datatypes.JSONMap{"size": "1024x1024", "format": "png", "quality": "standard", "promptHint": "keep hint"}}
		credential := &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"api_key": "must-not-be-saved", "base_url": "https://must-not-be-saved.invalid"}}
		err := s.SaveCredentialAndProfile(ctx, "dev", &tenant, credential, profile, true)
		require.Equal(t, "AI_MODEL_DEFAULTS_INVALID", dto.CodeOf(err), model)
		var app *dto.AppError
		require.ErrorAs(t, err, &app)
		require.Equal(t, 422, app.HTTPCode)
		require.Equal(t, "quality", app.Details["parameter"])
		require.EqualValues(t, []any{"auto", "low", "medium", "high"}, app.Details["allowed_values"])
		var after am.AIProviderCredential
		require.NoError(t, s.db.First(&after).Error)
		require.Equal(t, before.Data, after.Data)
		var count int64
		require.NoError(t, s.db.Model(&am.AIModelProfile{}).Count(&count).Error)
		require.Zero(t, count)
		require.NoError(t, s.db.Model(&am.AIRoutePolicy{}).Count(&count).Error)
		require.Zero(t, count)
		for _, quality := range []string{"", "low", "medium", "high"} {
			profile.Defaults["quality"] = quality
			credential.Data = datatypes.JSONMap{}
			require.NoError(t, s.SaveCredentialAndProfile(ctx, "dev", &tenant, credential, profile, false), model)
			want := quality
			if want == "" {
				want = "auto"
			}
			saved, err := s.GetProfile(ctx, "dev", &tenant, "image", "openai", model)
			require.NoError(t, err)
			require.Equal(t, want, saved.Defaults["quality"])
			require.Equal(t, "keep hint", saved.Defaults["promptHint"])
		}
		// 下一型号的拒绝断言仍从空画像／路由开始；不删除或重建凭据。
		require.NoError(t, s.db.Where("1=1").Delete(&am.AIModelProfile{}).Error)
	}
}

func TestAdminImageCatalogUsesTheSameOperationAsServiceCatalog(t *testing.T) {
	_, reg := onlineCatalogFixture(t)
	entries := []OnlineCatalogModel{{ID: "gpt-image-2"}, {ID: "gpt-image-2-2026-04-21"}, {ID: "dall-e-3"}}
	enrichImageOperations(reg, "openai", "", "image", entries)
	for _, entry := range entries {
		operation := entry.Operations[catalog.ImageGeneration]
		manifest := reg.ProtocolModel("image", "openai", "", entry.ID)
		require.Equal(t, manifest.Operations[catalog.ImageGeneration], operation)
		if entry.ID == "dall-e-3" {
			require.Contains(t, operation.Parameters["quality"].Enum, "standard")
		} else {
			require.Equal(t, "auto", operation.Parameters["quality"].Default)
			require.NotContains(t, operation.Parameters["quality"].Enum, "standard")
		}
	}
}

func TestImageQualityRepairIsScopedIdempotentAndRejectsStalePreview(t *testing.T) {
	s := environmentAliasFixture(t)
	tenant, other, ctx := onlineTestTenant, "22222222-2222-4222-8222-222222222222", context.Background()
	target := am.AIModelProfile{Env: "dev", TenantUUID: &tenant, Modality: "image", Provider: "openai", Model: "gpt-image-2", Defaults: datatypes.JSONMap{"quality": "standard", "size": "1024x1024", "format": "png", "promptHint": "preserve"}}
	require.NoError(t, s.db.Create(&target).Error)
	for _, profile := range []am.AIModelProfile{
		{Env: "prod", TenantUUID: &tenant, Modality: "image", Provider: "openai", Model: "gpt-image-2", Defaults: datatypes.JSONMap{"quality": "standard"}},
		{Env: "dev", TenantUUID: &other, Modality: "image", Provider: "openai", Model: "gpt-image-2", Defaults: datatypes.JSONMap{"quality": "high"}},
		{Env: "dev", TenantUUID: &tenant, Modality: "image", Provider: "openai", Model: "gpt-image-2-2026-04-21", Defaults: datatypes.JSONMap{"quality": "standard"}},
	} {
		require.NoError(t, s.db.Create(&profile).Error)
	}
	plan, err := s.PreviewImageQualityRepair(ctx, tenant)
	require.NoError(t, err)
	require.True(t, plan.Eligible)
	var before []am.AIModelProfile
	require.NoError(t, s.db.Order("id").Find(&before).Error)
	changed, err := s.ApplyImageQualityRepair(ctx, *plan)
	require.NoError(t, err)
	require.True(t, changed)
	changed, err = s.ApplyImageQualityRepair(ctx, *plan)
	require.NoError(t, err)
	require.False(t, changed)
	var after []am.AIModelProfile
	require.NoError(t, s.db.Order("id").Find(&after).Error)
	for i := range before {
		if before[i].ID == target.ID {
			before[i].Defaults["quality"] = "auto"
		}
		require.Equal(t, before[i].Defaults, after[i].Defaults)
	}
	current := datatypes.JSONMap{"quality": "standard", "size": "1024x1024", "format": "png"}
	require.NoError(t, s.db.Model(&target).Update("defaults", current).Error)
	plan, err = s.PreviewImageQualityRepair(ctx, tenant)
	require.NoError(t, err)
	current["quality"] = "high"
	require.NoError(t, s.db.Model(&target).Update("defaults", current).Error)
	changed, err = s.ApplyImageQualityRepair(ctx, *plan)
	require.ErrorContains(t, err, "AI_IMAGE_REPAIR_PREVIEW_STALE")
	require.False(t, changed)
	plan, err = s.PreviewImageQualityRepair(ctx, tenant)
	require.NoError(t, err)
	require.False(t, plan.Eligible, "合法显式 high 不参与修复")
}
