package capability_registry

import (
	"context"
	"testing"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestRecordMatchesFilters_Source(t *testing.T) {
	t.Parallel()

	corexRecord := models.CapabilityRecord{
		CapabilityID: "corex.capability.a",
		PluginID:     "corex.platform",
	}
	pluginRecord := models.CapabilityRecord{
		CapabilityID: "plugin.capability.b",
		PluginID:     "com.powerx.plugins.base.template",
	}

	// source empty means no source filtering (equivalent to source=all/any).
	if !recordMatchesFilters(corexRecord, CapabilityListOptions{Source: ""}) {
		t.Fatal("expected corex record to pass when source filter is empty")
	}
	if !recordMatchesFilters(pluginRecord, CapabilityListOptions{Source: ""}) {
		t.Fatal("expected plugin record to pass when source filter is empty")
	}

	if !recordMatchesFilters(corexRecord, CapabilityListOptions{Source: CapabilitySourceCoreX}) {
		t.Fatal("expected corex record to pass corex filter")
	}
	if recordMatchesFilters(pluginRecord, CapabilityListOptions{Source: CapabilitySourceCoreX}) {
		t.Fatal("expected plugin record to be filtered out by corex filter")
	}
	if !recordMatchesFilters(pluginRecord, CapabilityListOptions{Source: CapabilitySourcePlugin}) {
		t.Fatal("expected plugin record to pass plugin filter")
	}
	if recordMatchesFilters(corexRecord, CapabilityListOptions{Source: CapabilitySourcePlugin}) {
		t.Fatal("expected corex record to be filtered out by plugin filter")
	}
}

func TestListCapabilitiesAppliesSourceFilterBeforePagination(t *testing.T) {
	db := newMemoryDB(t)
	recordRepo := repo.NewCapabilityRecordRepository(db, nil)
	service := NewRegistryService(RegistryServiceOptions{
		RecordRepo:   recordRepo,
		TemplateRepo: repo.NewWorkflowTemplateRepository(db),
		JobRepo:      repo.NewCapabilitySyncJobRepository(db),
	})
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := recordRepo.Upsert(ctx, &models.CapabilityRecord{
			CapabilityID:     "corex.capability." + string(rune('a'+i)),
			PluginID:         "corex.platform",
			PluginVersion:    "1.0.0",
			Title:            "CoreX",
			Protocols:        datatypes.JSON([]byte(`[{"channel":"rest"}]`)),
			CapabilitiesHash: "hash-corex",
			ProtocolHash:     "protocol-corex",
			Status:           "published",
		})
		require.NoError(t, err)
	}
	for i := 0; i < 3; i++ {
		_, err := recordRepo.Upsert(ctx, &models.CapabilityRecord{
			CapabilityID:     "plugin.capability." + string(rune('a'+i)),
			PluginID:         "com.powerx.plugins.scrm",
			PluginVersion:    "0.1.0",
			Title:            "Plugin",
			Protocols:        datatypes.JSON([]byte(`[{"channel":"rest"}]`)),
			CapabilitiesHash: "hash-plugin",
			ProtocolHash:     "protocol-plugin",
			Status:           "published",
		})
		require.NoError(t, err)
	}

	items, total, err := service.ListCapabilities(ctx, CapabilityListOptions{
		Source:       CapabilitySourcePlugin,
		Limit:        2,
		Offset:       0,
		IncludeTotal: true,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, items, 2)
	for _, item := range items {
		require.Equal(t, CapabilitySourcePlugin, CapabilitySource(item.Record))
	}
}
