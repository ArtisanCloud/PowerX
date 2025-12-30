package setting

import (
	"context"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestListTenantPluginBindings(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	coremodel.PowerXSchema = "main"
	require.NoError(t, db.AutoMigrate(&dbsetting.PluginInstanceConfig{}))

	repo := NewPluginInstanceConfigRepository(db)
	ctx := context.Background()

	fixtures := []dbsetting.PluginInstanceConfig{
		{
			TenantUUID: "AEFFC79F-E72A-4FD9-B908-5C150BCE3741",
			PluginID:   "plugin.alpha",
			Key:        "auth.credentials",
			Enabled:    true,
		},
		{
			TenantUUID: "aeffc79f-e72a-4fd9-b908-5c150bce3741",
			PluginID:   "plugin.beta",
			Key:        "auth.credentials",
			Enabled:    true,
		},
		{
			TenantUUID: "BEFFC79F-E72A-4FD9-B908-5C150BCE1234",
			PluginID:   "plugin.alpha",
			Key:        "auth.credentials",
			Enabled:    true,
		},
		{
			TenantUUID: "aeffc79f-e72a-4fd9-b908-5c150bce3741",
			PluginID:   "plugin.gamma",
			Key:        "custom.setting",
			Enabled:    true,
		},
	}
	for _, rec := range fixtures {
		item := rec
		require.NoError(t, repo.Upsert(ctx, &item))
	}

	require.NoError(t, repo.SetEnabled(ctx, "beffc79f-e72a-4fd9-b908-5c150bce1234", "plugin.alpha", false))

	opts := ListTenantPluginOptions{
		Key:         "auth.credentials",
		OnlyEnabled: true,
	}
	rows, err := repo.ListTenantPluginBindings(ctx, opts)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	opts = ListTenantPluginOptions{
		TenantUUIDs: []string{"AEFFC79F-E72A-4FD9-B908-5C150BCE3741"},
		PluginIDs:   []string{"plugin.alpha"},
		Key:         "auth.credentials",
		OnlyEnabled: true,
	}
	rows, err = repo.ListTenantPluginBindings(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, []TenantPluginBinding{
		{TenantUUID: "aeffc79f-e72a-4fd9-b908-5c150bce3741", PluginID: "plugin.alpha"},
	}, rows)

	opts = ListTenantPluginOptions{
		PluginIDs:   []string{"plugin.gamma"},
		OnlyEnabled: true,
	}
	rows, err = repo.ListTenantPluginBindings(ctx, opts)
	require.NoError(t, err)
	require.Equal(t, []TenantPluginBinding{
		{TenantUUID: "aeffc79f-e72a-4fd9-b908-5c150bce3741", PluginID: "plugin.gamma"},
	}, rows)
}
