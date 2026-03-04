package capabilityregistry

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func capabilityRegistryContext(t testing.TB, parent context.Context, tenantUUID string) context.Context {
	t.Helper()
	if parent == nil {
		parent = context.Background()
	}
	if tenantUUID == "" {
		tenantUUID = defaultTenantUUID
	}
	md := metadata.New(map[string]string{
		"tenant-uuid": tenantUUID,
		"authorization": "Bearer contract",
	})
	return metadata.NewOutgoingContext(parent, md)
}

const defaultTenantUUID = "8a21845e-d1b6-4df1-b2ce-1d3bde3b8a03"

func assertNoCapabilityRegistryTenantLeak(t testing.TB, msg proto.Message) {
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
