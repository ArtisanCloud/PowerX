package capability_registry

import (
	"context"
	"fmt"
	"testing"

	gatewayv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/integration_gateway/v1"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type capacityRejectedInvoker struct{}

func (capacityRejectedInvoker) Invoke(context.Context, capservice.InvocationInput) (capservice.InvocationResult, error) {
	return capservice.InvocationResult{}, fmt.Errorf("%w: host_capacity_exhausted", capservice.ErrInvocationStorageCapacity)
}

func TestStorageCapacityRejectionPreservesGRPCContract(t *testing.T) {
	server := &IntegrationGatewayServer{selector: capservice.NewSelector(capservice.SelectorOptions{Invoker: capacityRejectedInvoker{}})}
	response, err := server.InvokeCapability(context.Background(), &gatewayv1.InvokeCapabilityRequest{CapabilityId: "com.corex.test", TenantUuid: "owned-tenant"})
	require.Nil(t, response)
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.Contains(t, status.Convert(err).Message(), "STORAGE_CAPACITY_EXHAUSTED")
}
