package knowledge_space_contract

import (
	"context"
	"net"
	"testing"
	"time"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestFusionGRPCHandlers(t *testing.T) {
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
	policyID := env.SeedPolicyTemplate("grpc-fusion", "v1")
	space := env.CreateSpaceFixture("grpc-fusion-space", policyID)

	rpcCtx := knowledgeGRPCContext(t, env)
	publishResp, err := client.PublishFusionStrategy(rpcCtx, &knowledgev1.FusionStrategyRequest{
		SpaceId:         space.UUID.String(),
		Label:           "baseline",
		Bm25Weight:      0.3,
		VectorWeight:    0.7,
		GraphConstraint: "tenant:default",
		RerankerModel:   "cross-encoder-v1",
		ConflictPolicy:  "allow_with_flag",
	})
	require.NoError(t, err)
	require.Equal(t, knowledgev1.FusionStrategy_DEPLOYMENT_STATE_ACTIVE, publishResp.GetStrategy().GetDeploymentState())
	firstID := publishResp.GetStrategy().GetStrategyId()
	assertNoLegacyTenantProto(t, publishResp)

	queueResp, err := client.PublishFusionStrategy(rpcCtx, &knowledgev1.FusionStrategyRequest{
		SpaceId:         space.UUID.String(),
		Label:           "queued",
		Bm25Weight:      0.5,
		VectorWeight:    0.5,
		GraphConstraint: "tenant:default",
		RerankerModel:   "cross-encoder-v1",
		ConflictPolicy:  "queue",
	})
	require.NoError(t, err)
	require.Equal(t, knowledgev1.FusionStrategy_DEPLOYMENT_STATE_DRAFT, queueResp.GetStrategy().GetDeploymentState())
	assertNoLegacyTenantProto(t, queueResp)

	listResp, err := client.ListFusionStrategies(rpcCtx, &knowledgev1.ListFusionStrategiesRequest{
		SpaceId: space.UUID.String(),
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(listResp.GetStrategies()), 2)
	assertNoLegacyTenantProto(t, listResp)

	rollbackResp, err := client.RollbackFusionStrategy(rpcCtx, &knowledgev1.RollbackFusionStrategyRequest{
		SpaceId:    space.UUID.String(),
		StrategyId: firstID,
	})
	require.NoError(t, err)
	require.Equal(t, firstID, rollbackResp.GetStrategy().GetStrategyId())
	require.Equal(t, knowledgev1.FusionStrategy_DEPLOYMENT_STATE_ACTIVE, rollbackResp.GetStrategy().GetDeploymentState())
	assertNoLegacyTenantProto(t, rollbackResp)

	_, err = client.RollbackFusionStrategy(rpcCtx, &knowledgev1.RollbackFusionStrategyRequest{
		SpaceId:    space.UUID.String(),
		StrategyId: 99999,
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	require.Equal(t, codes.NotFound, st.Code())
}
