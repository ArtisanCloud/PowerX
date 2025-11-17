package knowledge_space_contract

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
)

func TestQABridgeGRPCPlanAndSnapshot(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	tpl := env.SeedPolicyTemplate("qa-bridge-grpc", "v1")
	spaceA := env.CreateSpaceFixture("grpc-qa-alpha", tpl)
	spaceB := env.CreateSpaceFixture("grpc-qa-beta", tpl)
	require.NoError(t, env.ActivateSpace(spaceA.UUID))
	require.NoError(t, env.ActivateSpace(spaceB.UUID))

	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = listener.Close() })

	server := env.GRPCServer()
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := knowledgev1.NewKnowledgeSpaceQABridgeServiceClient(conn)

	planResp, err := client.PlanRetrieval(ctx, &knowledgev1.QARetrievalPlanRequest{
		TenantId:        env.TenantID().String(),
		Intent:          "供应商是否超限",
		DomainTags:      []string{"finance"},
		SessionId:       "grpc-session",
		LatencyBudgetMs: 1500,
	})
	require.NoError(t, err)
	require.Len(t, planResp.GetCandidateSpaces(), 2)
	require.Equal(t, spaceA.UUID.String(), planResp.GetCandidateSpaces()[0].GetSpaceId())
	require.Empty(t, planResp.GetCandidateSpaces()[0].GetDegradeReason())
	require.Equal(t, "hybrid", planResp.GetCandidateSpaces()[0].GetStrategy())

	// degrade scenario
	require.NoError(t, env.SetSpaceStatus(spaceB.UUID, "retired"))
	planResp, err = client.PlanRetrieval(ctx, &knowledgev1.QARetrievalPlanRequest{
		TenantId:        env.TenantID().String(),
		Intent:          "供应商是否超限",
		DomainTags:      []string{"finance"},
		SessionId:       "grpc-session",
		LatencyBudgetMs: 1500,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(planResp.GetCandidateSpaces()))
	require.NotEmpty(t, planResp.GetCandidateSpaces()[1].GetDegradeReason())

	snapshotResp, err := client.UpsertMemorySnapshot(ctx, &knowledgev1.QAMemorySnapshotRequest{
		TenantId:  env.TenantID().String(),
		SessionId: "grpc-session",
		Updates: []*knowledgev1.QAMemoryUpdate{
			{
				ChunkId:    "chunk-grpc-1",
				SpaceId:    spaceA.UUID.String(),
				Citations:  []string{"doc#1"},
				Status:     "answered",
				SourceType: "pdf",
				Confidence: 0.9,
			},
			{
				ChunkId:    "chunk-grpc-2",
				SpaceId:    spaceA.UUID.String(),
				Citations:  []string{"doc#2"},
				Status:     "stale",
				SourceType: "api",
				Confidence: 0.6,
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, snapshotResp.GetCitations(), 2)
	require.Equal(t, "chunk-grpc-1", snapshotResp.GetCitations()[0].GetChunkId())

	// Read without updates
	snapshotResp, err = client.UpsertMemorySnapshot(ctx, &knowledgev1.QAMemorySnapshotRequest{
		TenantId:  env.TenantID().String(),
		SessionId: "grpc-session",
	})
	require.NoError(t, err)
	require.Len(t, snapshotResp.GetCitations(), 2)
}
