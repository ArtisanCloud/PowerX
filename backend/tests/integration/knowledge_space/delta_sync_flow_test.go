package knowledge_space_integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	ksdelta "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/delta"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/stretchr/testify/require"
)

func TestDeltaSyncFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	svc := env.Deps.KnowledgeSpace.Delta
	require.NotNil(t, svc)

	tpl := env.SeedPolicyTemplate("delta-int", "v1")
	space := env.CreateSpaceFixture("delta-int-space", tpl)

	job, err := svc.StartJob(context.Background(), ksdelta.StartJobInput{
		SpaceID:    space.UUID,
		Source:     "handbook",
		PackageURI: "s3://delta/int.tar.gz",
	})
	require.NoError(t, err)
	require.NotNil(t, job)

	_, err = svc.StartJob(context.Background(), ksdelta.StartJobInput{
		SpaceID:    space.UUID,
		Source:     "handbook",
		PackageURI: "s3://delta/int.tar.gz",
	})
	require.ErrorIs(t, err, ksdelta.ErrJobConflict)

	job, err = svc.Publish(context.Background(), ksdelta.PublishJobInput{
		JobID:        job.UUID,
		Decision:     "partial",
		ApprovedBy:   "ops@powerx.dev",
		DiffAccuracy: 99,
		PartialRelease: true,
	})
	require.NoError(t, err)
	require.Equal(t, "published", job.Status)
	require.True(t, job.PartialRelease)
	require.NotNil(t, job.ApprovedAt)
	require.NotNil(t, job.PublishedAt)
	require.LessOrEqual(t, job.PublishedAt.Sub(job.CreatedAt), 30*time.Minute)

	job, err = svc.Rollback(context.Background(), ksdelta.RollbackInput{
		JobID:       job.UUID,
		Reason:      "regression",
		RequestedBy: "qa@powerx.dev",
	})
	require.NoError(t, err)
	require.Equal(t, "rolled_back", job.Status)

	require.FileExists(t, env.DeltaReportPath)
	require.FileExists(t, env.KnowledgeUpdateReportPath)

	blob, err := os.ReadFile(env.DeltaReportPath)
	require.NoError(t, err)
	var snapshot map[string]any
	require.NoError(t, json.Unmarshal(blob, &snapshot))
	require.Contains(t, snapshot, "jobId")

	var auditCount int64
	require.NoError(t, env.DB.Model(&models.AuditTrailEntry{}).
		Where("space_uuid = ? AND action LIKE ?", space.UUID, "delta.job.%").
		Count(&auditCount).Error)
	require.GreaterOrEqual(t, auditCount, int64(2))
}
