package pipeline

import (
	"testing"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestGateRunnerEnforcesCoverageThreshold(t *testing.T) {
	runner := NewGateRunner(GateRunnerOptions{
		MinCoveragePercent: 85,
	})
	candidate := &models.PluginReleaseCandidate{
		ReleaseNotes: "release with coverage metadata",
		CommitHash:   "abcdef123456",
		Labels:       datatypes.JSON([]byte(`{"coverage": "90"}`)),
	}
	report := runner.Run(nil, candidate)
	require.True(t, report.Passed)
	require.Equal(t, 1.0, report.Score["coverage_score"])

	lowCoverage := &models.PluginReleaseCandidate{
		ReleaseNotes: "docs ok",
		CommitHash:   "abcdef123456",
		Labels:       datatypes.JSON([]byte(`{"coverage": "70"}`)),
	}
	low := runner.Run(nil, lowCoverage)
	require.False(t, low.Passed)
	require.Less(t, low.Score["coverage_score"], 1.0)
	require.Equal(t, "COVERAGE_BELOW_THRESHOLD", low.Violations[len(low.Violations)-1].Code)
}
