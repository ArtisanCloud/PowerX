package integration_gateway

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	r "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestEventFabricServiceSeedIsStableAndHasIndependentExplicitPermissions(t *testing.T) {
	db := newCatalogUpgradeDB(t)
	require.NoError(t, db.AutoMigrate(&iam.Permission{}))
	row := &tenant.Tenant{Key: "event-fabric-contract", Name: "Event Fabric Contract", Status: 1}
	row.UUID = uuid.New()
	require.NoError(t, db.Create(row).Error)
	ids := []string{"com.corex.eventfabric.service_publish", "com.corex.eventfabric.service_consume"}
	seeder := catalogSeeder(db)
	seeder.capabilityIDs = ids
	ctx := context.Background()
	for n := 0; n < 2; n++ {
		require.NoError(t, apikeypermissions.EnsureUpgradePermissionsForCapabilities(ctx, db, ids))
		require.NoError(t, seeder.Ensure(ctx))
	}
	registry := r.NewCapabilityRegistryRepository(db)
	for _, id := range ids {
		reg, err := registry.GetLatest(ctx, nil, id, row.UUID.String())
		require.NoError(t, err)
		require.EqualValues(t, 1, reg.Version)
		require.Len(t, reg.Adapters, 1)
		require.Equal(t, "core_internal", reg.Adapters[0].TransportType)
		require.Equal(t, "core://event-fabric/service", reg.Adapters[0].Endpoint)
		require.Equal(t, []string{reg.Adapters[0].AdapterID}, reg.RoutingPolicy.FallbackSequence)
	}
	var rows []iam.Permission
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 2)
	seen := map[string]bool{}
	for _, permission := range rows {
		var meta struct {
			CapabilityID string `json:"capability_id"`
			Explicit     bool   `json:"api_key_explicit"`
			APIKey       struct {
				Scope, Action   string
				ResourceType    string `json:"resource_type"`
				ResourcePattern string `json:"resource_pattern"`
			} `json:"api_key"`
		}
		require.NoError(t, json.Unmarshal(permission.Meta, &meta))
		require.True(t, permission.AllowAPIKey)
		require.True(t, meta.Explicit)
		require.Equal(t, "capability", meta.APIKey.ResourceType)
		require.NotEmpty(t, meta.APIKey.Scope)
		require.NotEmpty(t, meta.APIKey.ResourcePattern)
		seen[meta.CapabilityID] = true
	}
	for _, id := range ids {
		require.True(t, seen[id])
	}
}
