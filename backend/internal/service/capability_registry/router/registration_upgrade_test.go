package router

import (
	"context"
	"testing"

	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type upgradeRegistry struct {
	registry.Repository
	current registry.Registration
}

func (r *upgradeRegistry) GetLatest(context.Context, *gorm.DB, string, string) (registry.Registration, error) {
	return r.current, nil
}

func TestRegistrationUpgradeFencesStickyHealthAndInactiveAdapters(t *testing.T) {
	in := InvokeRequest{TenantUUID: "tenant", CapabilityID: "catalog", PreferredProtocol: "core_internal", StickyKey: "plugin"}
	reg := registry.Registration{TenantUUID: in.TenantUUID, CapabilityID: in.CapabilityID, Version: 1, Adapters: []registry.AdapterEndpoint{
		{AdapterID: "old-http", TransportType: "http", Endpoint: "/admin/models", IsActive: true},
		{AdapterID: "core", TransportType: "core_internal", Endpoint: "core://ai/model-catalog", IsActive: true},
	}}
	repo := &upgradeRegistry{current: reg}
	svc := NewService(ServiceOptions{RegistryRepository: repo})
	svc.stickyMap[stickyKeyFor(in.TenantUUID, in.CapabilityID, in.StickyKey)] = "old-http"
	selected, err := svc.Invoke(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, "core", selected.AdapterID)
	key := routingKey(in.TenantUUID, in.CapabilityID)
	svc.adapterState[key] = map[string]healthState{"core": {status: "unhealthy"}}
	reg.Version++
	_, err = svc.Simulate(context.Background(), reg, in)
	require.ErrorIs(t, err, ErrNoAdapter)
	require.False(t, svc.isAdapterHealthy(key, "core"), "sandbox cannot reset the live router's health state")
	repo.current = reg
	selected, err = svc.Invoke(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, "core", selected.AdapterID)
	reg.Adapters[1].IsActive = false
	reg.FallbackPlan = &registry.FallbackPlan{StaticResponse: &registry.StaticResponse{Payload: map[string]any{"items": []any{}}}}
	repo.current = reg
	_, err = svc.Invoke(context.Background(), in)
	require.ErrorIs(t, err, ErrNoAdapter)
}
