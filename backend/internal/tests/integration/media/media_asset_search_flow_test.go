package media

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
)

type MediaSearchFilter struct {
	TenantID       string
	Keyword        string
	Tags           []string
	BusinessStatus string
	IncludeDeleted bool
	Page           int
	PageSize       int
}

type MediaSearchResult struct {
	UUID           string
	TenantID       string
	Name           string
	BusinessStatus string
	Tags           []string
	Deleted        bool
}

func (env *mediaIntegrationTestEnv) SeedSearchFixtures(ctx context.Context) error {
	tenantUUID := env.ensureTenant("tenant_a")
	assetA, err := env.service.CreateAsset(ctx, mediasvc.CreateAssetInput{
		TenantUUID: tenantUUID,
		Name:       "homepage-banner",
		Driver:     "local",
		Tags:       []string{"homepage"},
	})
	if err != nil {
		return err
	}
	env.assetTenants[assetA.UUID] = tenantUUID
	env.setFixture("draft", assetA.UUID)

	assetB, err := env.service.CreateAsset(ctx, mediasvc.CreateAssetInput{
		TenantUUID: tenantUUID,
		Name:       "archived-banner",
		Driver:     "local",
		Tags:       []string{"homepage"},
	})
	if err != nil {
		return err
	}
	env.assetTenants[assetB.UUID] = tenantUUID
	env.setFixture("archived", assetB.UUID)

	statusArchived := "archived"
	if _, err := env.service.UpdateAsset(ctx, mediasvc.UpdateAssetInput{
		TenantUUID:     tenantUUID,
		UUID:           assetB.UUID,
		BusinessStatus: &statusArchived,
	}); err != nil {
		return err
	}
	return nil
}

func (env *mediaIntegrationTestEnv) SearchAssets(ctx context.Context, filter MediaSearchFilter) ([]MediaSearchResult, uint64, error) {
	tenantUUID := env.ensureTenant(filter.TenantID)
	input := mediasvc.ListAssetsInput{
		TenantUUID:     tenantUUID,
		Keyword:        filter.Keyword,
		TagsAll:        filter.Tags,
		IncludeDeleted: filter.IncludeDeleted,
		Page:           filter.Page,
		PageSize:       filter.PageSize,
	}
	if filter.BusinessStatus != "" {
		input.BusinessStatus = []string{filter.BusinessStatus}
	}
	assets, total, err := env.service.ListAssets(ctx, input)
	if err != nil {
		return nil, 0, err
	}
	results := make([]MediaSearchResult, 0, len(assets))
	for _, asset := range assets {
		results = append(results, MediaSearchResult{
			UUID:           asset.UUID,
			TenantID:       filter.TenantID,
			Name:           asset.Name,
			BusinessStatus: asset.BusinessStatus,
			Tags:           append([]string(nil), asset.Tags...),
			Deleted:        asset.Deleted,
		})
	}
	return results, uint64(total), nil
}

func (env *mediaIntegrationTestEnv) SoftDeleteAsset(ctx context.Context, uuid string) error {
	tenantUUID := env.assetTenants[uuid]
	if tenantUUID == "" {
		return ErrMediaAssetNotFound
	}
	return env.service.DeleteAsset(ctx, mediasvc.DeleteAssetInput{
		TenantUUID: tenantUUID,
		UUID:       uuid,
	})
}

func TestMediaAssetSearchFlowFiltersAndSoftDelete(t *testing.T) {
	t.Parallel()

	env := newMediaIntegrationTestEnv(t)
	ctx := context.Background()

	require.NoError(t, env.SeedSearchFixtures(ctx))

	results, total, err := env.SearchAssets(ctx, MediaSearchFilter{
		TenantID: "tenant_a",
		Tags:     []string{"homepage"},
		Page:     1,
		PageSize: 20,
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, results, 2)

	// 验证结果包含一个 draft 状态的资源（不依赖顺序）
	var hasDraft bool
	var hasArchived bool
	for _, asset := range results {
		if asset.BusinessStatus == "draft" {
			hasDraft = true
		}
		if asset.BusinessStatus == "archived" {
			hasArchived = true
		}
	}
	require.True(t, hasDraft, "expected at least one asset with draft status")
	require.True(t, hasArchived, "expected at least one asset with archived status")

	require.NoError(t, env.SoftDeleteAsset(ctx, env.fixture("archived")))

	activeOnly, activeTotal, err := env.SearchAssets(ctx, MediaSearchFilter{
		TenantID:       "tenant_a",
		IncludeDeleted: false,
		Page:           1,
		PageSize:       20,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, activeTotal)
	require.Len(t, activeOnly, 1)

	deleted, deletedTotal, err := env.SearchAssets(ctx, MediaSearchFilter{
		TenantID:       "tenant_a",
		IncludeDeleted: true,
		Page:           1,
		PageSize:       20,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, deletedTotal, uint64(2))
	require.Len(t, deleted, int(deletedTotal))
	foundDeleted := false
	for _, item := range deleted {
		if item.Deleted {
			foundDeleted = true
			break
		}
	}
	require.True(t, foundDeleted)
}
