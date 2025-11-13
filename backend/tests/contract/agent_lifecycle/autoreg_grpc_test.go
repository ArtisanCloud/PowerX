//go:build ignore

package agentlifecyclecontract

import (
	"context"
	"net"
	"testing"
	"time"

	agentv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"
	agentgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/agentlifecycle"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestAutoRegisterManifestGRPC(t *testing.T) {
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

	resp, err := client.RegisterManifest(ctx, &agentv1.RegisterManifestRequest{
		PluginId:                 "plugins.demo.analytics",
		PluginVersion:            "1.0.0",
		ManifestVersion:          "2025-03-01",
		TenantId:                 "tenant-grpc",
		Alias:                    "grpc-agent",
		TelemetryContractVersion: "otel-agent-v1",
		ToolGrants: []*agentv1.ToolGrant{
			{Name: "summarize", Version: "v1"},
		},
		DefaultCapacityInstances: 2,
		SandboxProfile:           "smoke",
		Signature:                "valid-signature",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "grpc-agent", resp.Agent.GetAlias())
	require.NotNil(t, resp.Sandbox)
	require.Equal(t, "completed", resp.Sandbox.GetStatus())

	runResp, err := client.RunSandbox(ctx, &agentv1.RunSandboxRequest{
		AgentId: resp.Agent.GetId(),
		Profile: "smoke",
	})
	require.NoError(t, err)
	require.Equal(t, "completed", runResp.Sandbox.GetStatus())

	_, err = client.RegisterManifest(ctx, &agentv1.RegisterManifestRequest{
		PluginId:                 "plugins.demo.analytics",
		PluginVersion:            "1.0.0",
		ManifestVersion:          "2025-03-01",
		TenantId:                 "tenant-grpc",
		Alias:                    "grpc-agent-2",
		TelemetryContractVersion: "otel-agent-v1",
		Signature:                "invalid",
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	require.Equal(t, codes.PermissionDenied, st.Code())
}
