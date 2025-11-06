package plugin_release

import (
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	"github.com/ArtisanCloud/PowerX/internal/transport/grpc/plugin_release"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

func TestPluginReleaseGRPC_ReleaseGuardrailRPCs(t *testing.T) {
	deps, cleanup := setupPluginReleaseDeps(t)
	defer cleanup()

	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = listener.Close() })

	server := grpc.NewServer()
	plugin_release.RegisterServer(server, deps)
	go func() {
		if err := server.Serve(listener); err != nil {
			t.Errorf("gRPC server failed: %v", err)
		}
	}()
	t.Cleanup(server.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	client := pb.NewPluginReleaseServiceClient(conn)
	callCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer test")

	candidateResp, err := client.CreateReleaseCandidate(callCtx, &pb.CreateReleaseCandidateRequest{
		TenantId:         "tenant-rc",
		PluginId:         "plugin.demo",
		Version:          "0.1.0",
		BuildArtifactUri: "s3://bucket/build.tar.gz",
		CommitHash:       "abc1234",
		ReleaseNotes:     "first release",
	})
	require.NoError(t, err)
	require.Equal(t, "tenant-rc", candidateResp.GetTenantId())
	require.Equal(t, "plugin.demo", candidateResp.GetPluginId())

	gateResp, err := client.RunQualityGates(callCtx, &pb.RunQualityGatesRequest{
		CandidateId: candidateResp.GetCandidateId(),
	})
	require.NoError(t, err)
	require.Equal(t, candidateResp.GetCandidateId(), gateResp.GetCandidateId())

	planResp, err := client.GenerateReleasePlan(callCtx, &pb.GenerateReleasePlanRequest{
		CandidateId: candidateResp.GetCandidateId(),
		WindowStart: time.Now().Add(time.Hour).Format(time.RFC3339),
		WindowEnd:   time.Now().Add(2 * time.Hour).Format(time.RFC3339),
		Batches: []*pb.CanaryBatch{
			{Name: "batch-1", TenantScope: []string{"tenant-a"}},
		},
	})
	require.NoError(t, err)
	require.Equal(t, candidateResp.GetCandidateId(), planResp.GetCandidateId())
	require.NotEmpty(t, planResp.GetPlanId())
}
