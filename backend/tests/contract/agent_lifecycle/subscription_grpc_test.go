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
	"google.golang.org/grpc/test/bufconn"
)

func TestSubscriptionGRPC(t *testing.T) {
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
		TenantId:                 "tenant-003",
		Alias:                    "subscription-grpc",
		TelemetryContractVersion: "otel-agent-v1",
	})
	require.NoError(t, err)
	agentID := registerResp.Agent.GetId()

	resp, err := client.UpdateSubscription(ctx, &agentv1.UpdateSubscriptionRequest{
		AgentId:  agentID,
		TenantId: "tenant-003",
		Config: &agentv1.SubscriptionConfig{
			MetricsFilter:  []string{"error_rate", "p95_latency_ms"},
			HealthStatuses: []string{"degraded", "unavailable"},
		},
	})
	require.NoError(t, err)
	firstUpdated, err := time.Parse(time.RFC3339, resp.GetConfig().GetUpdatedAt())
	require.NoError(t, err)
	require.True(t, firstUpdated.After(time.Time{}))

	getResp, err := client.GetSubscription(ctx, &agentv1.GetSubscriptionRequest{AgentId: agentID})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"error_rate", "p95_latency_ms"}, getResp.Config.GetMetricsFilter())
	require.ElementsMatch(t, []string{"degraded", "unavailable"}, getResp.Config.GetHealthStatuses())

	// 调整为仅在不可用状态告警
	updateResp, err := client.UpdateSubscription(ctx, &agentv1.UpdateSubscriptionRequest{
		AgentId:  agentID,
		TenantId: "tenant-003",
		Config: &agentv1.SubscriptionConfig{
			MetricsFilter:  []string{"success_rate"},
			HealthStatuses: []string{"unavailable"},
		},
	})
	require.NoError(t, err)
	updatedAt, err := time.Parse(time.RFC3339, updateResp.GetConfig().GetUpdatedAt())
	require.NoError(t, err)
	require.True(t, updatedAt.After(firstUpdated))

	// 非法配置应返回错误且保留原设置
	_, err = client.UpdateSubscription(ctx, &agentv1.UpdateSubscriptionRequest{
		AgentId:  agentID,
		TenantId: "tenant-003",
		Config:   &agentv1.SubscriptionConfig{},
	})
	require.Error(t, err)

	finalCfg, err := client.GetSubscription(ctx, &agentv1.GetSubscriptionRequest{AgentId: agentID})
	require.NoError(t, err)
	require.Equal(t, []string{"success_rate"}, finalCfg.Config.GetMetricsFilter())
	require.Equal(t, []string{"unavailable"}, finalCfg.Config.GetHealthStatuses())
}
