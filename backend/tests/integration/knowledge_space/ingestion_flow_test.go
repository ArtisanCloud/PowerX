package knowledge_space_integration

import (
	"context"
	"testing"

	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIngestionFlowPersistsArtifactsAndVectors(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	ctx := context.Background()
	policyID := env.SeedPolicyTemplate("integration-ingestion", "v1")
	space := env.CreateSpaceFixture("integration-space", policyID)

	ingestionSvc := env.Deps.KnowledgeSpace.Ingestion
	require.NotNil(t, ingestionSvc)

	job, err := ingestionSvc.Trigger(ctx, ksvc.TriggerIngestionInput{
		SpaceID:    space.UUID,
		SourceType: "pdf",
		SourceURI:  "s3://bucket/handbook.pdf",
		Priority:   "high",
	})
	require.NoError(t, err)
	require.Equal(t, models.IngestionStatusCompleted, job.Status)
	require.Greater(t, job.ChunkTotal, 0)
	require.NotNil(t, job.ArtifactBundleID)

	var bundle models.ArtifactBundle
	require.NoError(t, env.DB.WithContext(ctx).Where("id = ?", *job.ArtifactBundleID).Take(&bundle).Error)
	require.Equal(t, job.SummaryChunkCount, bundle.SummaryChunkCount)
	require.Equal(t, job.ParagraphChunkCount, bundle.ParagraphChunkCount)

	records := env.VectorStore.Records(space.UUID)
	require.Equal(t, job.ChunkTotal, len(records))

	chunkIDs := []uuid.UUID{records[0].ChunkID, records[1].ChunkID}
	require.NoError(t, ingestionSvc.RemoveChunks(ctx, space.UUID, chunkIDs))
	require.Equal(t, job.ChunkTotal-len(chunkIDs), len(env.VectorStore.Records(space.UUID)))

	spaceSvc := env.Deps.KnowledgeSpace.Service
	_, err = spaceSvc.RetireSpace(ctx, ksvc.RetireSpaceInput{
		SpaceID: space.UUID,
		Reason:  "integration cleanup",
	})
	require.NoError(t, err)
	require.Len(t, env.VectorStore.Records(space.UUID), 0)
}
