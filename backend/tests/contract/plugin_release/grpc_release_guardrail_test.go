//go:build ignore

package plugin_release

import (
	"context"
	"net"
	"testing"
	"time"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	"github.com/ArtisanCloud/PowerX/internal/transport/grpc/plugin_release"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestPluginReleaseGRPC_GuardrailLifecycle(t *testing.T) {
	deps, _ := setupPluginReleaseDeps(t)

	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = listener.Close() })

	server := grpc.NewServer()
	plugin_release.RegisterServer(server, deps)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := pluginreleasepb.NewPluginReleaseServiceClient(conn)

	createResp, err := client.CreateReleaseCandidate(ctx, &pluginreleasepb.CreateReleaseCandidateRequest{
		TenantId:         "tenant-test",
		PluginId:         "px.demo",
		Version:          "v1.0.0",
		BuildArtifactUri: "s3://bucket/releases/v1.0.0.zip",
		CommitHash:       "abcdef123456",
		ReleaseNotes:     "Initial GA release with automated coverage, security scan and rollback hooks.",
		Labels: map[string]string{
			"channel":  "beta",
			"coverage": "95",
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, createResp.GetCandidateId())
	require.Equal(t, "px.demo", createResp.GetPluginId())

	gateResp, err := client.RunQualityGates(ctx, &pluginreleasepb.RunQualityGatesRequest{
		CandidateId: createResp.GetCandidateId(),
	})
	require.NoError(t, err)
	require.Equal(t, createResp.GetCandidateId(), gateResp.GetCandidateId())
	require.Equal(t, "passed", gateResp.GetStatus())

	windowStart := time.Now().Add(1 * time.Hour).UTC()
	windowEnd := windowStart.Add(2 * time.Hour)
	planResp, err := client.GenerateReleasePlan(ctx, &pluginreleasepb.GenerateReleasePlanRequest{
		CandidateId: createResp.GetCandidateId(),
		WindowStart: windowStart.Format(time.RFC3339),
		WindowEnd:   windowEnd.Format(time.RFC3339),
		Batches: []*pluginreleasepb.CanaryBatch{
			{
				Name:        "batch-a",
				TenantScope: []string{"tenant-a"},
				MetricThresholds: map[string]float64{
					"error_rate": 0.02,
				},
				RollbackTimeoutMinutes: 15,
			},
		},
		RollbackScripts:     []string{"rollback.sh"},
		NotificationTargets: []string{"release@powerx.dev"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, planResp.GetPlanId())
	require.Equal(t, createResp.GetCandidateId(), planResp.GetCandidateId())
	require.Equal(t, "draft", planResp.GetStatus())
	require.Len(t, planResp.GetBatches(), 1)
	require.Equal(t, "batch-a", planResp.GetBatches()[0].GetName())
}
