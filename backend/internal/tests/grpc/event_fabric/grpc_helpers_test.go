package eventfabric

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const eventFabricGRPCTenantUUID = "tenant-corex"

func eventFabricGRPCContext(t testing.TB, parent context.Context, tenantUUID string) context.Context {
	t.Helper()
	if parent == nil {
		parent = context.Background()
	}
	if strings.TrimSpace(tenantUUID) == "" {
		tenantUUID = eventFabricGRPCTenantUUID
	}
	md := metadata.New(map[string]string{
		"x-tenant-uuid": tenantUUID,
		"authorization": "Bearer admin",
	})
	return metadata.NewOutgoingContext(parent, md)
}

func assertNoEventFabricTenantLeakProto(t testing.TB, msg proto.Message) {
	t.Helper()
	if msg == nil {
		return
	}
	data, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
	require.NoError(t, err)
	body := strings.ToLower(string(data))
	require.NotContains(t, body, "tenant_id", "grpc payload leaked tenant_id")
	require.NotContains(t, body, "tenantid", "grpc payload leaked tenantId")
}
