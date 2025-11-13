package knowledge_space_integration

import (
	"context"
	"testing"

	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestFusionStrategyFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	ctx := context.Background()
	policyID := env.SeedPolicyTemplate("fusion-integration", "v1")
	space := env.CreateSpaceFixture("fusion-integration-space", policyID)

	fusionSvc := env.Deps.KnowledgeSpace.Fusion
	require.NotNil(t, fusionSvc)

	active, err := fusionSvc.PublishStrategy(ctx, ksvc.PublishStrategyInput{
		SpaceID:         space.UUID,
		Label:           "baseline",
		BM25Weight:      0.35,
		VectorWeight:    0.65,
		GraphConstraint: "tenant:default",
		RerankerModel:   "cross-encoder-v1",
		ConflictPolicy:  "allow_with_flag",
	})
	require.NoError(t, err)
	require.Equal(t, models.FusionDeploymentActive, active.DeploymentState)

	queued, err := fusionSvc.PublishStrategy(ctx, ksvc.PublishStrategyInput{
		SpaceID:         space.UUID,
		Label:           "queued",
		BM25Weight:      0.5,
		VectorWeight:    0.5,
		GraphConstraint: "tenant:default",
		RerankerModel:   "cross-encoder-v1",
		ConflictPolicy:  "queue",
	})
	require.NoError(t, err)
	require.Equal(t, models.FusionDeploymentDraft, queued.DeploymentState)

	rolled, err := fusionSvc.RollbackStrategy(ctx, ksvc.RollbackStrategyInput{
		SpaceID:    space.UUID,
		StrategyID: active.ID,
	})
	require.NoError(t, err)
	require.Equal(t, models.FusionDeploymentActive, rolled.DeploymentState)

	matchID := uuid.New()
	env.VectorStore.SetQueryResponse(space.UUID, vectorstore.QueryResponse{
		Matches: []vectorstore.QueryMatch{
			{
				ChunkID:  matchID,
				Score:    0.84,
				Metadata: map[string]any{"chunk_kind": "summary"},
			},
		},
	})

	queryResult, err := fusionSvc.Query(ctx, ksvc.FusionQueryInput{
		SpaceID:   space.UUID,
		Embedding: []float32{0.1, 0.2, 0.3},
		TopK:      5,
	})
	require.NoError(t, err)
	require.Len(t, queryResult.Matches, 1)
	require.Equal(t, matchID, queryResult.Matches[0].ChunkID)
	require.Greater(t, queryResult.Matches[0].Score, 0.0)
	lastQuery := env.VectorStore.LastQuery()
	require.Equal(t, space.UUID, lastQuery.SpaceID)
	require.Equal(t, 5, lastQuery.TopK)
}
