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
	"google.golang.org/grpc/test/bufconn"
)

func TestAgentLifecycleGRPC(t *testing.T) {
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

	dialer := func(ctx context.Context, s string) (net.Conn, error) {
		return listener.Dial()
	}

	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(dialer), grpc.WithInsecure())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := agentv1.NewAgentLifecycleServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	registerResp, err := client.RegisterAgent(ctx, &agentv1.RegisterAgentRequest{
		TenantId:                 "tenant-001",
		Alias:                    "writer-grpc",
		TelemetryContractVersion: "otel-agent-v1",
		ToolGrants: []*agentv1.ToolGrant{
			{Name: "summarize", Version: "v1"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, registerResp)
	require.NotEmpty(t, registerResp.Agent.GetId())
	require.Equal(t, agentv1.AgentStatus_AGENT_STATUS_PENDING, registerResp.Agent.GetStatus())

	agentID := registerResp.Agent.GetId()

	getResp, err := client.GetAgent(ctx, &agentv1.GetAgentRequest{AgentId: agentID})
	require.NoError(t, err)
	require.Equal(t, agentID, getResp.Agent.GetId())
	require.Equal(t, agentv1.AgentStatus_AGENT_STATUS_PENDING, getResp.Agent.GetStatus())

	activateResp, err := client.ActivateAgent(ctx, &agentv1.LifecycleCommandRequest{
		AgentId:     agentID,
		Reason:      "initial rollout",
		RequestedBy: "grpc-admin",
	})
	require.NoError(t, err)
	require.NotNil(t, activateResp)

	getAfter, err := client.GetAgent(ctx, &agentv1.GetAgentRequest{AgentId: agentID})
	require.NoError(t, err)
	require.Equal(t, agentv1.AgentStatus_AGENT_STATUS_ACTIVE, getAfter.Agent.GetStatus())

	pauseResp, err := client.PauseAgent(ctx, &agentv1.LifecycleCommandRequest{
		AgentId:     agentID,
		Reason:      "maintenance",
		RequestedBy: "grpc-admin",
	})
	require.NoError(t, err)
	require.NotNil(t, pauseResp.GetEvent())
	require.Equal(t, agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_PAUSE, pauseResp.Event.GetType())

	resumeResp, err := client.ResumeAgent(ctx, &agentv1.LifecycleCommandRequest{
		AgentId:     agentID,
		RequestedBy: "grpc-admin",
	})
	require.NoError(t, err)
	require.Equal(t, agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_RESUME, resumeResp.Event.GetType())

	scaleResp, err := client.ScaleAgent(ctx, &agentv1.ScaleAgentRequest{
		AgentId:                 agentID,
		TargetCapacityInstances: 7,
		RequestedBy:             "grpc-admin",
	})
	require.NoError(t, err)
	require.Equal(t, agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_SCALE_UP, scaleResp.Event.GetType())
	require.Equal(t, int32(7), scaleResp.Event.GetRequestedCapacity())

	retireResp, err := client.RetireAgent(ctx, &agentv1.LifecycleCommandRequest{
		AgentId:     agentID,
		Reason:      "sunset",
		RequestedBy: "grpc-admin",
	})
	require.NoError(t, err)
	require.Equal(t, agentv1.LifecycleEventType_LIFECYCLE_EVENT_TYPE_RETIRE, retireResp.Event.GetType())

	finalResp, err := client.GetAgent(ctx, &agentv1.GetAgentRequest{AgentId: agentID})
	require.NoError(t, err)
	require.Equal(t, agentv1.AgentStatus_AGENT_STATUS_RETIRED, finalResp.Agent.GetStatus())
}
