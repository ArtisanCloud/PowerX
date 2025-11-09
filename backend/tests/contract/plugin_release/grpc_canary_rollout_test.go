//go:build ignore

package plugin_release

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	grpcserver "github.com/ArtisanCloud/PowerX/internal/transport/grpc/plugin_release"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestPluginReleaseGRPC_CanaryRolloutLifecycle(t *testing.T) {
	deps, _ := setupPluginReleaseDeps(t)

	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = listener.Close() })

	server := grpc.NewServer()
	grpcserver.RegisterServer(server, deps)
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
		TenantId:         "tenant-canary",
		PluginId:         "px.demo",
		Version:          "v2.1.0",
		BuildArtifactUri: "s3://bucket/releases/v2.1.0.zip",
		CommitHash:       "commitabcdef",
		ReleaseNotes:     "Production rollout smoke test release.",
		Labels: map[string]string{
			"coverage": "95",
		},
	})
	require.NoError(t, err)

	_, err = client.RunQualityGates(ctx, &pluginreleasepb.RunQualityGatesRequest{
		CandidateId: createResp.GetCandidateId(),
	})
	require.NoError(t, err)

	windowStart := time.Now().Add(30 * time.Minute).UTC()
	windowEnd := windowStart.Add(time.Hour)
	planResp, err := client.GenerateReleasePlan(ctx, &pluginreleasepb.GenerateReleasePlanRequest{
		CandidateId: createResp.GetCandidateId(),
		WindowStart: windowStart.Format(time.RFC3339),
		WindowEnd:   windowEnd.Format(time.RFC3339),
		Batches: []*pluginreleasepb.CanaryBatch{
			{
				Name:        "batch-safe",
				TenantScope: []string{"tenant-east"},
				MetricThresholds: map[string]float64{
					"error_rate": 0.05,
				},
				RollbackTimeoutMinutes: 10,
			},
		},
		RollbackScripts:     []string{"rollback.sh"},
		NotificationTargets: []string{"release@powerx.dev"},
	})
	require.NoError(t, err)

	stream, err := client.TriggerCanary(ctx, &pluginreleasepb.TriggerCanaryRequest{
		PlanId:    planResp.GetPlanId(),
		BatchName: "batch-safe",
	})
	require.NoError(t, err)

	progressCount := 0
	for {
		progress, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		require.NoError(t, recvErr)
		progressCount++
		require.Equal(t, "batch-safe", progress.GetBatchName())
	}
	require.GreaterOrEqual(t, progressCount, 2)

	finalResp, err := client.FinalizeDeployment(ctx, &pluginreleasepb.FinalizeDeploymentRequest{
		PlanId: planResp.GetPlanId(),
		Action: "promote",
	})
	require.NoError(t, err)
	require.Equal(t, planResp.GetPlanId(), finalResp.GetPlanId())
	require.Equal(t, "completed", strings.ToLower(finalResp.GetFinalState()))
}
