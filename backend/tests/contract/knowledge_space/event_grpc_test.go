package knowledge_space_contract

import (
	"context"
	"net"
	"testing"
	"time"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestEventHotfixGRPC(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	server := env.GRPCServer()
	lis := bufconn.Listen(1024 * 1024)
	go func() {
		_ = server.Serve(lis)
	}()
	t.Cleanup(func() { server.Stop() })

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithInsecure(),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	client := knowledgev1.NewKnowledgeSpaceAdminServiceClient(conn)
	received := timestamppb.New(time.Now())
	ctx := knowledgeGRPCContext(t, env)
	applyResp, err := client.ApplyEvent(ctx, &knowledgev1.ApplyEventRequest{
		EventId:    "evt-grpc-1",
		EventType:  "policy-update",
		Payload:    map[string]string{"tenant": env.TenantUUID().String()},
		ReceivedAt: received,
	})
	require.NoError(t, err)
	require.Equal(t, "applied", applyResp.GetStatus())
	assertNoLegacyTenantProto(t, applyResp)

	retryResp, err := client.RetryEvent(ctx, &knowledgev1.RetryEventRequest{
		EventId:    "evt-grpc-2",
		EventType:  "policy-update",
		Payload:    map[string]string{"tenant": env.TenantUUID().String()},
		ReceivedAt: received,
		RetryCount: 1,
	})
	require.NoError(t, err)
	require.Equal(t, "applied", retryResp.GetStatus())
	assertNoLegacyTenantProto(t, retryResp)

	_, err = client.ApplyEvent(ctx, &knowledgev1.ApplyEventRequest{
		EventId:   "evt-grpc-1",
		EventType: "policy-update",
	})
	require.Error(t, err)

	_, err = client.ApplyEvent(ctx, &knowledgev1.ApplyEventRequest{
		EventId:   "evt-grpc-1",
		EventType: "policy-update",
		Payload:   map[string]string{"tenant": env.TenantUUID().String()},
		ReceivedAt: received,
	})
	require.Error(t, err)
}
