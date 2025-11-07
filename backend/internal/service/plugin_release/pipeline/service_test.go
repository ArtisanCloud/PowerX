package pipeline

import (
	"context"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPipelineGeneratesPlanAfterGates(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.AutoMigrate(&models.PluginReleaseCandidate{}, &models.ReleasePlan{}, &models.CanaryDeploymentRecord{}))

	candidateRepo := repo.NewReleaseCandidateRepository(db)
	planRepo := repo.NewReleasePlanRepository(db)
	pipelineSvc := NewService(candidateRepo, planRepo, NewGateRunner(GateRunnerOptions{RequireCommitHash: true}), Options{})
	require.NotNil(t, pipelineSvc)

	candidate, err := pipelineSvc.SubmitCandidate(context.Background(), SubmitCandidateInput{
		TenantID:      "tenant-test",
		PluginID:      "px.demo",
		Version:       "v1.0.0",
		BuildArtifact: "s3://bucket/plugins/v1.0.0.zip",
		CommitHash:    "abcdef123456",
		ReleaseNotes:  "Release note describing QA coverage, rollback plan and sign-off.",
		Labels: map[string]string{
			"channel":  "beta",
			"coverage": "95",
		},
		Actor:         "unittest",
	})
	require.NoError(t, err)
	require.Equal(t, models.PluginReleaseGateStatusPending, candidate.GateStatus)

	gateResult, err := pipelineSvc.RunQualityGates(context.Background(), RunQualityGatesInput{
		CandidateID: candidate.UUID,
	})
	require.NoError(t, err)
	require.Equal(t, models.PluginReleaseGateStatusPassed, gateResult.Status)

	stored, err := pipelineSvc.GetCandidate(context.Background(), candidate.UUID)
	require.NoError(t, err)
	require.Equal(t, models.PluginReleaseGateStatusPassed, stored.GateStatus)

	windowStart := time.Now().Add(30 * time.Minute)
	windowEnd := windowStart.Add(time.Hour)
	plan, updatedCandidate, err := pipelineSvc.GenerateReleasePlan(context.Background(), GeneratePlanInput{
		CandidateID: candidate.UUID,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		CanaryBatches: []CanaryBatchInput{
			{
				Name:        "batch-a",
				TenantScope: []string{"tenant-a"},
			},
		},
		RollbackScripts: []string{"rollback.sh"},
	})
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Equal(t, models.ReleasePlanStatusDraft, plan.Status)
	require.Equal(t, models.PluginReleaseApprovalApproved, updatedCandidate.ApprovalStatus)
}

func TestSubmitCandidateValidation(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.AutoMigrate(&models.PluginReleaseCandidate{}))

	candidateRepo := repo.NewReleaseCandidateRepository(db)
	planRepo := repo.NewReleasePlanRepository(db)

	svc := NewService(candidateRepo, planRepo, nil, Options{})
	_, err = svc.SubmitCandidate(context.Background(), SubmitCandidateInput{
		TenantID:      "tenant-id",
		PluginID:      "",
		Version:       "v1",
		BuildArtifact: "s3://bucket/artifact.zip",
		CommitHash:    "abc",
		ReleaseNotes:  "notes",
	})
	require.Error(t, err)
	require.Equal(t, ErrInvalidInput, err)
}
