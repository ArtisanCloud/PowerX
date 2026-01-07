package knowledge_space_contract

import (
	"context"
	"net"
	"testing"
	"time"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestTriggerIngestionGRPC(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = listener.Close() })

	server := env.GRPCServer()
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

	client := knowledgev1.NewKnowledgeSpaceAdminServiceClient(conn)
	policyID := env.SeedPolicyTemplate("grpc-ingestion", "v1")
	space := env.CreateSpaceFixture("grpc-ingest", policyID)

	rpcCtx := knowledgeGRPCContext(t, env)
	resp, err := client.TriggerIngestion(rpcCtx, &knowledgev1.IngestionJobRequest{
		SpaceId:   space.UUID.String(),
		Format:    "markdown",
		SourceUri: "https://example.com/wiki.md",
		Priority:  "normal",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.GetJob())
	require.Equal(t, "completed", resp.GetJob().GetStatus())
	require.Greater(t, resp.GetJob().GetChunkTotal(), uint32(0))
	require.Equal(t, uint32(0), resp.GetJob().GetRetryCount())
	assertNoLegacyTenantProto(t, resp)

	env.VectorStore.SetUpsertFailures(1)
	retryResp, err := client.TriggerIngestion(rpcCtx, &knowledgev1.IngestionJobRequest{
		SpaceId:   space.UUID.String(),
		Format:    "pdf",
		SourceUri: "s3://bucket/retry.pdf",
	})
	require.NoError(t, err)
	require.Equal(t, uint32(1), retryResp.GetJob().GetRetryCount())

	blockResp, err := client.TriggerIngestion(rpcCtx, &knowledgev1.IngestionJobRequest{
		SpaceId:     space.UUID.String(),
		Format:      "pdf",
		SourceUri:   "s3://bucket/scan-required.pdf",
		OcrRequired: true,
	})
	require.NoError(t, err)
	require.Equal(t, "blocked", blockResp.GetJob().GetStatus())
	require.Equal(t, "ocr_required", blockResp.GetJob().GetErrorCode())

	degradedResp, err := client.TriggerIngestion(rpcCtx, &knowledgev1.IngestionJobRequest{
		SpaceId:   space.UUID.String(),
		Format:    "pdf",
		SourceUri: "s3://bucket/scan.pdf",
	})
	require.NoError(t, err)
	require.Equal(t, "completed", degradedResp.GetJob().GetStatus())
	require.Equal(t, "degraded", degradedResp.GetJob().GetErrorCode())

	_, err = client.TriggerIngestion(rpcCtx, &knowledgev1.IngestionJobRequest{
		SpaceId:   uuid.New().String(),
		Format:    "pdf",
		SourceUri: "s3://bucket/missing.pdf",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.NotFound, st.Code())
}
