package media

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
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
	return errors.New("search fixtures seeding not implemented")
}

func (env *mediaIntegrationTestEnv) SearchAssets(ctx context.Context, filter MediaSearchFilter) ([]MediaSearchResult, uint64, error) {
	return nil, 0, errors.New("media search flow not implemented")
}

func (env *mediaIntegrationTestEnv) SoftDeleteAsset(ctx context.Context, uuid string) error {
	return errors.New("soft delete simulation not implemented")
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
	require.Equal(t, "draft", results[0].BusinessStatus)

	require.NoError(t, env.SoftDeleteAsset(ctx, "mas_archived"))

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
	require.True(t, deleted[0].Deleted)
}
