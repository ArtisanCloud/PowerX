package integrationgatewaycontract

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func integrationGatewayGRPCContext(t testing.TB, tenantUUID string) context.Context {
	t.Helper()
	base := context.Background()
	md := metadata.New(nil)
	if tenantUUID != "" {
		md.Set("x-tenant-uuid", strings.TrimSpace(tenantUUID))
	}
	md.Set("authorization", "Bearer token")
	return metadata.NewOutgoingContext(base, md)
}

func assertIGNoLegacyProto(t testing.TB, msg proto.Message) {
	t.Helper()
	if msg == nil {
		return
	}
	data, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
	require.NoError(t, err)
	payload := strings.ToLower(string(data))
	require.NotContains(t, payload, "tenant_id", "grpc payload leaked tenant_id")
	require.NotContains(t, payload, "tenantid", "grpc payload leaked tenantId")
}
