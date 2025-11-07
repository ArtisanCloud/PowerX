package pluginreleaseintegration

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/pipeline"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/stretchr/testify/require"
)

func TestReleaseGuardrailPipelineFlow(t *testing.T) {
	env := newPluginReleaseEnv(t)
	pipelineSvc := env.Service.Pipeline()
	require.NotNil(t, pipelineSvc)

	candidate, err := pipelineSvc.SubmitCandidate(context.Background(), pipeline.SubmitCandidateInput{
		TenantID:      "tenant-integration",
		PluginID:      "px.demo.integration",
		Version:       "v3.0.0",
		BuildArtifact: "s3://bucket/releases/v3.0.0.zip",
		CommitHash:    "commit1234567890",
		ReleaseNotes:  "Integration test release with automated QA, rollback drills and security gates.",
		Labels:        map[string]string{"channel": "integration"},
		Actor:         "integration-test",
	})
	require.NoError(t, err)
	require.Equal(t, models.PluginReleaseGateStatusPending, candidate.GateStatus)
	require.Equal(t, models.PluginReleaseApprovalSubmitted, candidate.ApprovalStatus)

	gateResult, err := pipelineSvc.RunQualityGates(context.Background(), pipeline.RunQualityGatesInput{
		CandidateID: candidate.UUID,
		Actor:       "integration-test",
	})
	require.NoError(t, err)
	require.Equal(t, models.PluginReleaseGateStatusPassed, gateResult.Status)

	windowStart := time.Now().Add(3 * time.Hour).UTC()
	windowEnd := windowStart.Add(2 * time.Hour)
	plan, updatedCandidate, err := pipelineSvc.GenerateReleasePlan(context.Background(), pipeline.GeneratePlanInput{
		CandidateID: candidate.UUID,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		CanaryBatches: []pipeline.CanaryBatchInput{
			{
				Name:                "batch-1",
				TenantScope:         []string{"tenant-east"},
				MetricThresholds:    map[string]float64{"error_rate": 0.02},
				RollbackTimeoutMins: 10,
			},
		},
		RollbackScripts:     []string{"bin/rollback.sh"},
		NotificationTargets: []string{"release-oncall@powerx.dev"},
		DependencyList:      []string{"px.runtime>=1.2.0"},
		Actor:               "release-manager",
	})
	require.NoError(t, err)
	require.Equal(t, models.ReleasePlanStatusDraft, plan.Status)
	require.Equal(t, candidate.ID, plan.ReleaseCandidateID)
	require.Equal(t, models.PluginReleaseApprovalApproved, updatedCandidate.ApprovalStatus)
	require.Equal(t, models.PluginReleaseGateStatusPassed, updatedCandidate.GateStatus)
}
