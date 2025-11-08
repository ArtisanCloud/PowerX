package pluginreleaseintegration

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/pipeline"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/runtime"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/stretchr/testify/require"
)

func TestCanaryRollbackFlow(t *testing.T) {
	env := newPluginReleaseEnv(t)
	pipelineSvc := env.Service.Pipeline()
	runtimeSvc := env.Service.Runtime()
	require.NotNil(t, runtimeSvc)

	ctx := context.Background()
	candidate, err := pipelineSvc.SubmitCandidate(ctx, pipeline.SubmitCandidateInput{
		TenantID:      "tenant-runtime",
		PluginID:      "px.demo.runtime",
		Version:       "v4.0.0",
		BuildArtifact: "s3://bucket/releases/v4.0.0.zip",
		CommitHash:    "runtimecommit",
		ReleaseNotes:  "Runtime rollback integration test release.",
	})
	require.NoError(t, err)
	_, err = pipelineSvc.RunQualityGates(ctx, pipeline.RunQualityGatesInput{CandidateID: candidate.UUID})
	require.NoError(t, err)

	plan, _, err := pipelineSvc.GenerateReleasePlan(ctx, pipeline.GeneratePlanInput{
		CandidateID: candidate.UUID,
		WindowStart: time.Now().Add(10 * time.Minute),
		WindowEnd:   time.Now().Add(40 * time.Minute),
		CanaryBatches: []pipeline.CanaryBatchInput{
			{
				Name:        "batch-critical",
				TenantScope: []string{"tenant-critical"},
				MetricThresholds: map[string]float64{
					"error_rate": 0.005,
				},
				RollbackTimeoutMins: 5,
			},
		},
		RollbackScripts:     []string{"rollback.sh"},
		NotificationTargets: []string{"release@powerx.dev"},
	})
	require.NoError(t, err)

	events, err := runtimeSvc.TriggerCanary(ctx, runtime.TriggerCanaryInput{
		PlanID:    plan.ID,
		BatchName: "batch-critical",
		Actor:     "integration-test",
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(events), 2)
	require.True(t, events[len(events)-1].ThresholdBreached)

	finalPlan, err := runtimeSvc.FinalizeDeployment(ctx, runtime.FinalizeInput{
		PlanID: plan.ID,
		Action: "rollback",
		Actor:  "integration-test",
	})
	require.NoError(t, err)
	require.Equal(t, models.ReleasePlanStatusRolledBack, finalPlan.Status)
}
