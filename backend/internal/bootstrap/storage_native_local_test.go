package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
)

// Explicit local metadata-only acceptance: no worker, network listener,
// notification delivery, production connection or source/policy mutation.
func TestStorageNativeLocalObservationAcceptance(t *testing.T) {
	path := os.Getenv("POWERX_STORAGE_NATIVE_LOCAL_ACCEPTANCE_CONFIG")
	if path == "" {
		t.Skip("opt-in configured local native log observation")
	}
	require.True(t, filepath.IsAbs(path))
	t.Chdir(filepath.Dir(filepath.Dir(path)))
	cfg, err := config.LoadForMigration(path)
	require.NoError(t, err)
	require.Equal(t, "dev", cfg.Deployment.Env)
	require.NotEmpty(t, cfg.EnvironmentID)
	require.NotEmpty(t, cfg.StorageGovernance.NotificationTenantUUID)
	require.NotEmpty(t, cfg.StorageGovernance.NotificationMemberUUID)
	var registered archiveops.Source
	for _, source := range registeredArchiveLogs(cfg) {
		if source.ID == "core-postgres-native" {
			registered = source
		}
	}
	require.Equal(t, "postgresql", registered.RotationFormat)
	require.Equal(t, "postgresql_logs", registered.Kind)
	require.NotEmpty(t, registered.CurrentLogfiles)
	// Use an isolated ledger, never the daily lifecycle/budget state.
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	cfg.StorageGovernance.RuntimeDirectory = filepath.Join(root, "runtime")
	cfg.StorageGovernance.SamplingEnabled = false // this acceptance never writes daily database samples
	cfg.StorageGovernance.PostgresEnabled = false // SQL/cluster/recipient preflight is a separate read-only check
	deps := &shared.Deps{}
	require.NoError(t, configureStorageGovernance(cfg, deps, archiveops.Config{Directory: filepath.Join(root, "archive")}, nil, filepath.Join(root, "backup")))
	t.Cleanup(func() { budget.SetDefault(nil) })
	ctx := context.Background()
	status, err := deps.StorageGovernance.Runtime(reqctx.WithIsRoot(reqctx.WithClaims(ctx, &reqctx.CoreXClaims{UserID: 1, IsRoot: true}), true))
	require.NoError(t, err)
	var native lifecycle.Source
	for _, source := range status.Lifecycle.Sources {
		if source.ID == "core-postgres-native" {
			native = source
		}
	}
	require.True(t, native.Supported, native.Problem)
	require.Equal(t, "shared", native.Scope)
	require.Empty(t, native.InstanceID, "shared cluster logs must not be attributed exclusively to this environment")
	require.Equal(t, registered.CurrentLogfiles, native.CurrentLogfiles)
	require.Empty(t, status.Lifecycle.Policies)
	plan, err := lifecycle.InspectSealedSource(ctx, native)
	require.NoError(t, err)
	require.True(t, plan.Complete, plan.Problem)
	require.Positive(t, plan.Protected["active_or_unknown"])
	report, err := json.Marshal(map[string]any{"source_id": native.ID, "scope": native.Scope, "supported": native.Supported, "scan_complete": plan.Complete, "observed_allocated_bytes": plan.ObservedBytes, "stable_sealed_candidates": len(plan.Candidates), "protected": plan.Protected, "policies_activated": false, "notification_sent": false, "daily_ledgers_changed": false})
	require.NoError(t, err)
	t.Log(string(report))
}
