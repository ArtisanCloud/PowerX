package agentlifecycle_unit

import (
	"testing"
	"time"

	agentinstr "github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle/instrumentation"
	"github.com/stretchr/testify/require"
)

func TestAlertTrackerCooldown(t *testing.T) {
	tracker := agentinstr.NewAlertTracker(2 * time.Minute)
	now := time.Now()

	require.True(t, tracker.Try("Agent-001", "DEGRADED", now), "first alert should pass")
	require.False(t, tracker.Try("agent-001", "degraded", now.Add(30*time.Second)), "cooldown should block repeated alert")
	require.True(t, tracker.Try("Agent-001", "degraded", now.Add(3*time.Minute)), "after cooldown alert should pass again")
	require.True(t, tracker.Try("Agent-001", "unavailable", now.Add(3*time.Minute)), "different status should not share cooldown")
}
