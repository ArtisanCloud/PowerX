package capability_registry

import (
	"context"
	"errors"
	"testing"

	gatewayv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/integration_gateway/v1"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type storageAdmissionRejectedInvoker struct{ err error }

func (i storageAdmissionRejectedInvoker) Invoke(context.Context, capservice.InvocationInput) (capservice.InvocationResult, error) {
	return capservice.InvocationResult{}, i.err
}

func TestStorageAdmissionGRPCPreservesStructuredReasonAndTrace(t *testing.T) {
	for _, code := range []string{"STORAGE_CONTROL_SCHEMA_NOT_READY", "STORAGE_GOVERNANCE_UNAVAILABLE", "STORAGE_CAPACITY_EXHAUSTED"} {
		t.Run(code, func(t *testing.T) {
			failure := dto.NewErrorWithCode(503, code, "private-password", errors.New("private-sql"))
			server := &IntegrationGatewayServer{selector: capservice.NewSelector(capservice.SelectorOptions{Invoker: storageAdmissionRejectedInvoker{failure}})}
			response, err := server.InvokeCapability(context.Background(), &gatewayv1.InvokeCapabilityRequest{CapabilityId: "com.corex.test", TenantUuid: "owned-tenant", TraceId: "storage-owned-trace"})
			require.Nil(t, response)
			require.Equal(t, codes.Unavailable, status.Code(err))
			require.Contains(t, err.Error(), code)
			require.NotContains(t, err.Error(), "private-")
			var info *errdetails.ErrorInfo
			for _, detail := range status.Convert(err).Details() {
				if value, ok := detail.(*errdetails.ErrorInfo); ok {
					info = value
				}
			}
			require.NotNil(t, info)
			require.Equal(t, code, info.Reason)
			require.Equal(t, "storage-owned-trace", info.Metadata["trace_id"])
		})
	}
}
