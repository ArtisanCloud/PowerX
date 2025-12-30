package workflowcontract

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func workflowGRPCContext(t testing.TB, tenantUUID string) context.Context {
	t.Helper()
	if tenantUUID == "" {
		tenantUUID = "workflow-grpc-demo"
	}
	md := metadata.New(map[string]string{
		"x-tenant-uuid": tenantUUID,
		"authorization": "Bearer token",
	})
	return metadata.NewOutgoingContext(context.Background(), md)
}

func assertNoWorkflowTenantLeakProto(t testing.TB, msg proto.Message) {
	t.Helper()
	if msg == nil {
		return
	}
	data, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
	require.NoError(t, err)
	body := strings.ToLower(string(data))
	require.NotContains(t, body, "tenant_id", "grpc response leaked tenant_id")
	require.NotContains(t, body, "tenantid", "grpc response leaked tenantId")
}
