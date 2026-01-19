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

func TestQABridgeGRPCContract(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	policyID := env.SeedPolicyTemplate("grpc-qa-bridge", "v1")
	space := env.CreateSpaceFixture("grpc-qa-space", policyID)
	require.NoError(t, env.ActivateSpace(space.UUID))

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

	plan, err := client.PlanRetrieval(ctx, &knowledgev1.QARetrievalPlanRequest{
		TenantUuid:      env.TenantUUID().String(),
		Intent:          "grpc plan",
		DomainTags:      []string{"ops"},
		SessionId:       "grpc-session",
		LatencyBudgetMs: 1200,
	})
	require.NoError(t, err)
	require.Equal(t, env.TenantUUID().String(), plan.GetTenantUuid())
	require.NotEmpty(t, plan.GetTelemetry().GetTraceId())
	require.NotEmpty(t, plan.GetPolicyVersionSnapshot())
	require.NotEmpty(t, plan.GetStages())
	require.GreaterOrEqual(t, int(plan.GetDegradeCount()), 0)

	snap, err := client.UpsertMemorySnapshot(ctx, &knowledgev1.QAMemorySnapshotRequest{
		TenantUuid: env.TenantUUID().String(),
		SessionId:  "grpc-session",
		TraceId:    plan.GetTelemetry().GetTraceId(),
		Updates: []*knowledgev1.QAMemoryUpdate{{
			ChunkId:    "grpc-chunk-1",
			SpaceId:    space.UUID.String(),
			Citations:  []string{"doc#grpc"},
			Status:     "answered",
			SourceType: "md",
			Confidence: 0.9,
		}},
	})
	require.NoError(t, err)
	require.Len(t, snap.GetCitations(), 1)
	require.NotNil(t, snap.GetMetadata())
}

