package router

import (
	"context"
	"testing"

	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	"github.com/stretchr/testify/require"
)

func TestSelectAdapterTreatsRESTAndHTTPAsSameTransport(t *testing.T) {
	svc := &Service{adapterState: map[string]map[string]healthState{}}
	selection, err := svc.selectAdapter(context.Background(), registry.Registration{
		Adapters: []registry.AdapterEndpoint{{
			AdapterID:     "catalog-http",
			TransportType: "http",
			Endpoint:      "/api/v1/admin/agents/providers",
			IsActive:      true,
		}},
	}, InvokeRequest{PreferredProtocol: "rest"})
	require.NoError(t, err)
	require.Equal(t, "catalog-http", selection.adapterID)
	require.Equal(t, "http", selection.transport)
}
