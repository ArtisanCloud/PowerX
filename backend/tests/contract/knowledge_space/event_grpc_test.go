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
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestEventHotfixGRPC(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	server := env.GRPCServer()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go server.Serve(lis)
	t.Cleanup(func() { server.Stop() })

	conn, err := grpc.DialContext(context.Background(), lis.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	client := knowledgev1.NewKnowledgeSpaceAdminServiceClient(conn)
	received := timestamppb.New(time.Now())
	applyResp, err := client.ApplyEvent(context.Background(), &knowledgev1.ApplyEventRequest{
		EventId:    "evt-grpc-1",
		EventType:  "policy-update",
		Payload:    map[string]string{"tenant": env.TenantID().String()},
		ReceivedAt: received,
	})
	require.NoError(t, err)
	require.Equal(t, "applied", applyResp.GetStatus())

	retryResp, err := client.RetryEvent(context.Background(), &knowledgev1.RetryEventRequest{
		EventId:    "evt-grpc-2",
		EventType:  "policy-update",
		Payload:    map[string]string{"tenant": env.TenantID().String()},
		ReceivedAt: received,
		RetryCount: 1,
	})
	require.NoError(t, err)
	require.Equal(t, "applied", retryResp.GetStatus())

	_, err = client.ApplyEvent(context.Background(), &knowledgev1.ApplyEventRequest{
		EventId:   "evt-grpc-1",
		EventType: "policy-update",
	})
	require.Error(t, err)
}
