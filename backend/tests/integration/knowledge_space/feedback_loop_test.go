package knowledge_space_integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFeedbackLoopSchedulesReprocess(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)
	env.EnableFeedbackReprocessWorker()

	ctx := context.Background()
	policyID := env.SeedPolicyTemplate("feedback-integration", "v1")
	space := env.CreateSpaceFixture("feedback-space", policyID)

	svc := env.Deps.KnowledgeSpace.Feedback
	require.NotNil(t, svc)

	chunkID := uuid.New()
	caseModel, err := svc.SubmitFeedback(ctx, ksvc.SubmitFeedbackInput{
		SpaceID:      space.UUID,
		ReportedBy:   "qa@powerx.local",
		Severity:     "medium",
		IssueType:    "freshness",
		LinkedChunks: []uuid.UUID{chunkID},
		Notes:        "内容已经过期",
	})
	require.NoError(t, err)
	require.NotNil(t, caseModel)
	require.Equal(t, models.FeedbackStatusInProgress, caseModel.Status)
	require.NotNil(t, caseModel.ReprocessJobID)

	require.NotNil(t, env.Pipeline)
	last := env.Pipeline.LastInput()
	require.Equal(t, space.UUID, last.SpaceID)
	require.Equal(t, caseModel.UUID, last.CaseID)
	require.Equal(t, chunkID, last.ChunkIDs[0])

	var storedChunks []string
	require.NoError(t, json.Unmarshal(caseModel.LinkedChunks, &storedChunks))
	require.Contains(t, storedChunks, chunkID.String())

	require.FileExists(t, env.FeedbackReportPath)
	metricsBlob, err := os.ReadFile(env.FeedbackReportPath)
	require.NoError(t, err)
	var metrics ksvc.FeedbackMetrics
	require.NoError(t, json.Unmarshal(metricsBlob, &metrics))
	require.GreaterOrEqual(t, metrics.Backlog, 1)
	require.NotZero(t, metrics.RecordedAt.Unix())
	require.FileExists(t, env.KnowledgeUpdateReportPath)

	require.Eventually(t, func() bool {
		got, err := repo.NewFeedbackCaseRepository(env.DB).GetByUUID(ctx, caseModel.UUID.String(), nil)
		if err != nil || got == nil {
			return false
		}
		return got.Status == models.FeedbackStatusReprocessed && got.ClosedAt != nil
	}, 2*time.Second, 50*time.Millisecond)

	records := env.VectorStore.Records(space.UUID)
	require.NotEmpty(t, records)

	artifactBundle, err := findLatestBundle(ctx, env.DB, space.UUID)
	require.NoError(t, err)
	require.NotNil(t, artifactBundle)
	require.True(t, strings.HasPrefix(artifactBundle.ChunkManifestURI, "minio://"))
	require.FileExists(t, artifactURIToPath(t, artifactBundle.ChunkManifestURI))
	require.FileExists(t, artifactURIToPath(t, artifactBundle.VectorManifestURI))
	require.FileExists(t, artifactURIToPath(t, artifactBundle.GraphManifestURI))

	env.VectorStore.SetUpsertFailures(1)
	badChunk := uuid.New()
	badCase, err := svc.SubmitFeedback(ctx, ksvc.SubmitFeedbackInput{
		SpaceID:      space.UUID,
		ReportedBy:   "qa@powerx.local",
		Severity:     "high",
		IssueType:    "accuracy",
		LinkedChunks: []uuid.UUID{badChunk},
		Notes:        "vector write should fail",
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		got, err := repo.NewFeedbackCaseRepository(env.DB).GetByUUID(ctx, badCase.UUID.String(), nil)
		if err != nil || got == nil {
			return false
		}
		return got.Status == models.FeedbackStatusEscalated && got.EscalatedAt != nil
	}, 2*time.Second, 50*time.Millisecond)
	require.Empty(t, findRecordByChunk(env.VectorStore.Records(space.UUID), badChunk))
}

func findLatestBundle(ctx context.Context, db *gorm.DB, space uuid.UUID) (*models.ArtifactBundle, error) {
	var job models.IngestionJob
	if err := db.WithContext(ctx).
		Where("space_uuid = ? AND source_type = ?", space, "reprocess").
		Order("created_at DESC").
		Limit(1).
		Take(&job).Error; err != nil {
		return nil, err
	}
	if job.ArtifactBundleID == nil {
		return nil, nil
	}
	return repo.NewArtifactBundleRepository(db).GetById(ctx, *job.ArtifactBundleID, nil)
}

func artifactURIToPath(t testing.TB, uri string) string {
	t.Helper()
	uri = strings.TrimSpace(uri)
	require.True(t, strings.HasPrefix(uri, "minio://"))
	trimmed := strings.TrimPrefix(uri, "minio://")
	parts := strings.SplitN(trimmed, "/", 2)
	require.Len(t, parts, 2)
	base := filepath.Join(testenv.ProjectRoot(t), "tmp", "knowledge-artifacts")
	return filepath.Join(base, parts[0], filepath.FromSlash(parts[1]))
}

func findRecordByChunk(records []vectorstore.VectorRecord, chunk uuid.UUID) []vectorstore.VectorRecord {
	out := make([]vectorstore.VectorRecord, 0, 1)
	for _, r := range records {
		if r.ChunkID == chunk {
			out = append(out, r)
		}
	}
	return out
}
