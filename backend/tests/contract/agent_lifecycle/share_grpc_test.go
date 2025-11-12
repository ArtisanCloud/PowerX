//go:build ignore

package agentlifecyclecontract

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	agentv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"
	agentgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/agentlifecycle"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestShareAgentGRPC(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	agentgrpc.Register(server, agentgrpc.NewServer(env.Deps.AgentLifecycle.Service))
	done := make(chan struct{})
	go func() {
		_ = server.Serve(listener)
		close(done)
	}()
	t.Cleanup(func() {
		server.GracefulStop()
		_ = listener.Close()
		<-done
	})

	dialer := func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(dialer), grpc.WithInsecure())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := agentv1.NewAgentLifecycleServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	agentID := env.SeedAgent("tenant-grpc-share", "grpc-share")

	req := &agentv1.CreateAgentShareRequest{
		AgentId:     agentID.String(),
		TenantId:    "tenant-target-grpc",
		RequestedBy: "ops-grpc",
		TraceId:     "trace-grpc-1",
		Quotas: []*agentv1.ShareQuota{
			{Type: "rpm", Limit: 1000},
		},
		Metadata: map[string]string{
			"region": "eu",
		},
	}

	resp, err := client.ShareAgent(ctx, req)
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetShareId())
	require.Equal(t, "tenant-target-grpc", resp.GetTenantId())
	require.Equal(t, "active", resp.GetStatus())

	env.ShareValidator.Err = errors.New("tenant not allowed")
	_, err = client.ShareAgent(ctx, &agentv1.CreateAgentShareRequest{
		AgentId:     agentID.String(),
		TenantId:    "tenant-denied",
		RequestedBy: "ops-grpc",
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	require.Equal(t, codes.InvalidArgument, st.Code())
	env.ShareValidator.Err = nil

	_, err = client.ShareAgent(ctx, req)
	require.Error(t, err)
	st, _ = status.FromError(err)
	require.Equal(t, codes.AlreadyExists, st.Code())

	revokeResp, err := client.RevokeAgentShare(ctx, &agentv1.RevokeAgentShareRequest{
		ShareId:     resp.GetShareId(),
		Reason:      "cleanup",
		RequestedBy: "ops-grpc",
	})
	require.NoError(t, err)
	require.Equal(t, "revoked", revokeResp.GetStatus())
	require.Equal(t, resp.GetShareId(), revokeResp.GetShareId())

	_, err = client.RevokeAgentShare(ctx, &agentv1.RevokeAgentShareRequest{
		ShareId: uuid.NewString(),
	})
	require.Error(t, err)
	st, _ = status.FromError(err)
	require.Equal(t, codes.NotFound, st.Code())
}
