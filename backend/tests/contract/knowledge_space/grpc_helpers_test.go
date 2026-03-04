package knowledge_space_contract

import (
	"context"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func knowledgeGRPCContext(t testing.TB, env *testenv.Env) context.Context {
	t.Helper()
	require.NotNil(t, env)
	tenantUUID := strings.TrimSpace(env.TenantUUID().String())
	require.NotEmpty(t, tenantUUID, "tenant uuid required for grpc context")
	md := metadata.New(map[string]string{
		"authorization": "Bearer token",
		"tenant-uuid": tenantUUID,
	})
	return metadata.NewOutgoingContext(context.Background(), md)
}

func assertNoLegacyTenantProto(t testing.TB, msg proto.Message) {
	t.Helper()
	if msg == nil {
		return
	}
	data, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
	require.NoError(t, err)
	body := strings.ToLower(string(data))
	require.NotContains(t, body, "tenant_id", "grpc response leaked tenant_id field")
	require.NotContains(t, body, "tenantid", "grpc response leaked tenantId field")
}
