package knowledge_space_integration

import (
	"context"
	"strings"
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

	cases := []struct {
		name           string
		format         string
		sourceURI      string
		ocrRequired    bool
		expectDegraded bool
	}{
		{name: "pdf-text", format: "pdf", sourceURI: "s3://bucket/handbook.pdf"},
		{name: "pdf-scan-degraded", format: "pdf", sourceURI: "s3://bucket/scan.pdf", expectDegraded: true},
		{name: "word", format: "docx", sourceURI: "s3://bucket/doc.docx"},
		{name: "csv", format: "csv", sourceURI: "s3://bucket/data.csv"},
		{name: "html", format: "html", sourceURI: "https://example.com/index.html"},
		{name: "sql", format: "sql", sourceURI: "s3://bucket/schema.sql"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job, err := ingestionSvc.Trigger(ctx, ksvc.TriggerIngestionInput{
				SpaceID:        space.UUID,
				Format:         tc.format,
				SourceURI:      tc.sourceURI,
				OCRRequired:    tc.ocrRequired,
				Priority:       "high",
				MaskingProfile: "masking.v1",
			})
			require.NoError(t, err)
			require.Equal(t, models.IngestionStatusCompleted, job.Status)
			require.Greater(t, job.ChunkTotal, 0)
			require.NotNil(t, job.ArtifactBundleID)
			require.Greater(t, job.SummaryChunkCount, 0)
			require.Greater(t, job.ParagraphChunkCount, 0)

			if tc.expectDegraded {
				require.Equal(t, "degraded", job.ErrorCode)
				require.Less(t, job.ChunkCoveredPct, 95.0)
			} else {
				require.Empty(t, job.ErrorCode)
				require.GreaterOrEqual(t, job.ChunkCoveredPct, 95.0)
			}

			var bundle models.ArtifactBundle
			require.NoError(t, env.DB.WithContext(ctx).Where("id = ?", *job.ArtifactBundleID).Take(&bundle).Error)
			require.True(t, strings.HasPrefix(bundle.ChunkManifestURI, "minio://"))
			require.True(t, strings.HasPrefix(bundle.VectorManifestURI, "minio://"))
			require.Len(t, bundle.Checksum, 64)
			require.Equal(t, job.SummaryChunkCount, bundle.SummaryChunkCount)
			require.Equal(t, job.ParagraphChunkCount, bundle.ParagraphChunkCount)

			records := env.VectorStore.Records(space.UUID)
			var kinds = map[string]bool{"doc_summary": false, "section_summary": false, "chunk": false}
			for _, rec := range records {
				if rec.Metadata["source_uri"] == tc.sourceURI {
					if kind, ok := rec.Metadata["chunk_kind"].(string); ok {
						kinds[kind] = true
					}
				}
			}
			require.True(t, kinds["doc_summary"])
			require.True(t, kinds["section_summary"])
			require.True(t, kinds["chunk"])
		})
	}

	records := env.VectorStore.Records(space.UUID)
	require.Greater(t, len(records), 0)
	chunkIDs := []uuid.UUID{records[0].ChunkID}
	require.NoError(t, ingestionSvc.RemoveChunks(ctx, space.UUID, chunkIDs))
	require.Equal(t, len(records)-len(chunkIDs), len(env.VectorStore.Records(space.UUID)))

	spaceSvc := env.Deps.KnowledgeSpace.Service
	_, err := spaceSvc.RetireSpace(ctx, ksvc.RetireSpaceInput{
		SpaceID: space.UUID,
		Reason:  "integration cleanup",
	})
	require.NoError(t, err)
	require.Len(t, env.VectorStore.Records(space.UUID), 0)
}
