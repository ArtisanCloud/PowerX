//go:build ignore

package agentlifecyclecontract

import (
	"context"
	"net"
	"testing"
	"time"

	agentv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	agentgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/agentlifecycle"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

func TestHealthGRPC(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	server := grpc.NewServer()
	agentgrpc.Register(server, agentgrpc.NewServer(env.Deps.AgentLifecycle.Service))

	listener := bufconn.Listen(1024 * 1024)
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

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.Dial()
	}
	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(dialer), grpc.WithInsecure())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := agentv1.NewAgentLifecycleServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	// 直接通过 service 注册代理，聚焦健康接口合同
	reg, err := env.Deps.AgentLifecycle.Service.Register(ctx, agent_lifecycle.RegisterInput{
		TenantID:                 "tenant-002",
		Alias:                    "health-grpc",
		TelemetryContractVersion: "otel-agent-v1",
	})
	require.NoError(t, err)
	agentID := reg.Agent.ID.String()

	record := func(offset time.Duration, status string, metrics agent_lifecycle.HealthMetricsInput) {
		err := env.Deps.AgentLifecycle.Service.RecordHealthSnapshot(ctx, agent_lifecycle.HealthInput{
			AgentID:         reg.Agent.ID,
			TenantID:        "tenant-002",
			WindowStartedAt: time.Now().Add(offset * -1),
			WindowDuration:  time.Minute,
			Status:          status,
			Metrics:         metrics,
		})
		require.NoError(t, err)
	}

	record(2*time.Hour, "healthy", agent_lifecycle.HealthMetricsInput{
		ThroughputPerMin: 150,
		SuccessRate:      0.99,
		P95LatencyMs:     100,
		ResourceUtilPct:  0.4,
		ErrorRate:        0.02,
	})

	record(30*time.Minute, "degraded", agent_lifecycle.HealthMetricsInput{
		ThroughputPerMin: 60,
		SuccessRate:      0.7,
		P95LatencyMs:     2200,
		ResourceUtilPct:  0.9,
		ErrorRate:        0.5,
		AnomalyTraceIDs:  []string{"trace-grpc"},
	})

	summaryResp, err := client.GetHealthSummary(ctx, &agentv1.GetHealthSummaryRequest{AgentId: agentID})
	require.NoError(t, err)
	require.Equal(t, agentv1.HealthStatus_HEALTH_STATUS_DEGRADED, summaryResp.GetSummary().GetStatus())
	require.NotEmpty(t, summaryResp.GetSummary().GetRecommendations())
	require.Greater(t, summaryResp.GetSummary().GetMetrics().GetErrorRate(), 0.4)

	historyResp, err := client.ListHealthSnapshots(ctx, &agentv1.ListHealthSnapshotsRequest{
		AgentId:       agentID,
		RangeHours:    3,
		WindowMinutes: 1,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(historyResp.GetSnapshots()), 2)
	require.Equal(t, agentv1.HealthStatus_HEALTH_STATUS_DEGRADED, historyResp.GetSnapshots()[0].GetStatus())
}
