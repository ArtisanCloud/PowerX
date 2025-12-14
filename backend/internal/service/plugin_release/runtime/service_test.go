package runtime

import (
	"context"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const runtimeTenantUUID = "9e8f2d3c-4578-41ab-9df0-bf556799acd2"

func TestTriggerCanaryRollsBackWhenThresholdBreached(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.AutoMigrate(
		&models.PluginReleaseCandidate{},
		&models.ReleasePlan{},
		&models.CanaryDeploymentRecord{},
	))

	candidateRepo := repo.NewReleaseCandidateRepository(db)
	planRepo := repo.NewReleasePlanRepository(db)

	candidate, err := candidateRepo.CreateCandidate(context.Background(), &models.PluginReleaseCandidate{
		TenantUUID:       runtimeTenantUUID,
		PluginID:         "px.demo",
		Version:          "v4.2.0",
		BuildArtifactURI: "s3://bucket/build.zip",
		CommitHash:       "abcdef123456",
		ReleaseNotes:     "runtime test",
		GateStatus:       models.PluginReleaseGateStatusPassed,
		ApprovalStatus:   models.PluginReleaseApprovalApproved,
	})
	require.NoError(t, err)

	truntime := NewService(Dependencies{
		Plans:      planRepo,
		Candidates: candidateRepo,
		Clock: func() time.Time {
			return time.Unix(0, 0)
		},
	}, Options{
		RollbackTimeout: 5 * time.Minute,
	})

	meta := datatypes.JSON([]byte(`{"metric_thresholds":{"error_rate":0.01}}`))
	plan, err := planRepo.CreatePlan(context.Background(), &models.ReleasePlan{
		ReleaseCandidateID: candidate.ID,
		WindowStart:        time.Now(),
		WindowEnd:          time.Now().Add(2 * time.Hour),
		Status:             models.ReleasePlanStatusScheduled,
	}, []*models.CanaryDeploymentRecord{
		{
			BatchName:      "batch-critical",
			MetricSnapshot: meta,
		},
	})
	require.NoError(t, err)

	events, err := truntime.TriggerCanary(context.Background(), TriggerCanaryInput{
		PlanID:    plan.ID,
		BatchName: "batch-critical",
		Actor:     "runtime-test",
	})
	require.NoError(t, err)
	require.Len(t, events, 3)
	require.True(t, events[len(events)-1].ThresholdBreached)

	updatedPlan, err := planRepo.GetPlanByID(context.Background(), plan.ID)
	require.NoError(t, err)
	require.Equal(t, models.ReleasePlanStatusRolledBack, updatedPlan.Status)

	finalized, err := truntime.FinalizeDeployment(context.Background(), FinalizeInput{
		PlanID: plan.ID,
		Action: "rollback",
		Actor:  "runtime-test",
	})
	require.NoError(t, err)
	require.Equal(t, models.ReleasePlanStatusRolledBack, finalized.Status)

}
