package knowledge_space

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFeedbackMetricsWriterRefresh(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() {
		coremodel.PowerXSchema = prevSchema
	})
	require.NoError(t, db.AutoMigrate(&models.FeedbackCase{}))

	now := time.Now().UTC()
	spaceID := uuid.New()

	closedAuto := &models.FeedbackCase{
		PowerUUIDModel: coremodel.PowerUUIDModel{
			UUID:      uuid.New(),
			CreatedAt: now.Add(-10 * time.Hour),
			UpdatedAt: now.Add(-10 * time.Hour),
		},
		SpaceUUID:      spaceID,
		ReportedBy:     "ops@powerx.dev",
		IssueType:      "accuracy",
		Severity:       models.FeedbackSeverityHigh,
		Status:         models.FeedbackStatusClosed,
		ClosedAt:       ptrTime(now),
		ReprocessJobID: ptrUint64(42),
	}
	require.NoError(t, db.Create(closedAuto).Error)

	closedManual := &models.FeedbackCase{
		PowerUUIDModel: coremodel.PowerUUIDModel{
			UUID:      uuid.New(),
			CreatedAt: now.Add(-6 * time.Hour),
			UpdatedAt: now.Add(-6 * time.Hour),
		},
		SpaceUUID:  spaceID,
		ReportedBy: "ops@powerx.dev",
		IssueType:  "freshness",
		Severity:   models.FeedbackSeverityMedium,
		Status:     models.FeedbackStatusReprocessed,
		ClosedAt:   ptrTime(now.Add(-time.Hour)),
	}
	require.NoError(t, db.Create(closedManual).Error)

	openCase := &models.FeedbackCase{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.New()},
		SpaceUUID:      spaceID,
		ReportedBy:     "ops@powerx.dev",
		IssueType:      "accuracy",
		Severity:       models.FeedbackSeverityLow,
		Status:         models.FeedbackStatusOpen,
	}
	require.NoError(t, db.Create(openCase).Error)

	dir := t.TempDir()
	feedbackPath := filepath.Join(dir, "knowledge-feedback.json")
	aggregatePath := filepath.Join(dir, "knowledge-update.json")
	w := NewFeedbackMetricsWriter(feedbackPath, aggregatePath)
	metrics, err := w.Refresh(ctx, db)
	require.NoError(t, err)
	require.Equal(t, 1, metrics.Backlog)
	require.Equal(t, 2, metrics.SampleSize)
	require.InDelta(t, 8.0, metrics.LoopTimeHours, 0.5)
	require.InDelta(t, 50.0, metrics.FixAccuracyPct, 0.1)
	require.InDelta(t, 50.0, metrics.AutoRatePct, 0.1)

	assertJSONFile(t, feedbackPath, metrics)
	assertJSONFile(t, aggregatePath, map[string]any{"feedback": metrics})
}

func ptrUint64(v uint64) *uint64 {
	return &v
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func assertJSONFile(t *testing.T, path string, expected interface{}) {
	t.Helper()
	require.FileExists(t, path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var decoded interface{}
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NotNil(t, decoded)
}
