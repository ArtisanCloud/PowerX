package knowledge_space_contract

import (
	"context"
	"net"
	"testing"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

func TestDeltaGRPCFlow(t *testing.T) {
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
	tpl := env.SeedPolicyTemplate("delta-grpc", "v1")
	space := env.CreateSpaceFixture("delta-grpc-space", tpl)

	ctx := knowledgeGRPCContext(t, env)
	startResp, err := client.StartDeltaJob(ctx, &knowledgev1.StartDeltaJobRequest{
		SpaceId:    space.UUID.String(),
		Source:     "handbook",
		PackageUri: "s3://delta/grpc.tgz",
	})
	require.NoError(t, err)
	require.NotNil(t, startResp.GetJob())
	assertNoLegacyTenantProto(t, startResp)
	jobID := startResp.GetJob().GetJobId()
	require.NotEmpty(t, jobID)

	_, err = client.StartDeltaJob(ctx, &knowledgev1.StartDeltaJobRequest{
		SpaceId:    space.UUID.String(),
		Source:     "handbook",
		PackageUri: "s3://delta/grpc.tgz",
	})
	require.Error(t, err)

	reportResp, err := client.GetDeltaReport(ctx, &knowledgev1.GetDeltaReportRequest{JobId: jobID})
	require.NoError(t, err)
	assertNoLegacyTenantProto(t, reportResp)

	publishResp, err := client.PublishDeltaJob(ctx, &knowledgev1.PublishDeltaJobRequest{
		JobId:        jobID,
		Decision:     "partial",
		DiffAccuracy: 97.5,
		PartialRelease: true,
	})
	require.NoError(t, err)
	require.Equal(t, "published", publishResp.GetJob().GetStatus())
	assertNoLegacyTenantProto(t, publishResp)

	rollbackResp, err := client.RollbackDelta(ctx, &knowledgev1.RollbackDeltaRequest{
		JobId:  jobID,
		Reason: "regression",
	})
	require.NoError(t, err)
	assertNoLegacyTenantProto(t, rollbackResp)

	_, err = client.PublishDeltaJob(ctx, &knowledgev1.PublishDeltaJobRequest{
		JobId:    uuid.New().String(),
		Decision: "publish",
	})
	require.Error(t, err)
}
