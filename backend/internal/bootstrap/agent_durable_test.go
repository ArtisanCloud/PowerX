package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentconfig "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"github.com/stretchr/testify/require"
)

func TestDurableSessionsStartupFailsClosed(t *testing.T) {
	cfg := &config.Config{}
	deps := &shared.Deps{}
	require.NoError(t, configureDurableSessions(context.Background(), cfg, deps, nil))
	cfg.AI.Runtime.DurableSessions = agentconfig.DurableSessions{Enabled: true, ReportBucket: "reports", WorkerConcurrency: 2, ScanInterval: time.Second, LeaseTTL: 30 * time.Second}
	require.ErrorContains(t, configureDurableSessions(context.Background(), cfg, deps, nil), "dependencies are required")
	require.Nil(t, deps.AgentRunWorker)
	require.Nil(t, deps.AgentSessionSvc)
	cfg.AI.Runtime.DurableSessions.WorkerConcurrency = 0
	require.ErrorContains(t, configureDurableSessions(context.Background(), cfg, deps, nil), "worker_concurrency")
}
