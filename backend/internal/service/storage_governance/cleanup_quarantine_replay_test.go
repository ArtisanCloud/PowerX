package storagegov

import (
	"context"
	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupQuarantineNativeSQLReplayKeepsHistoricalOutcomeAndNeedsFreshReview(t *testing.T) {
	if os.Getenv("POWERX_STORAGE_RETENTION_RESTORE_TEST") != "1" {
		t.Skip("owned native pg_dump/pg_restore only")
	}
	s, jobs, db, store, _, apply, f := resolutionSetup(t)
	ctx := controlRoot()
	damageQuarantineFixture(t, db, f, "missing")
	in := quarantineInput(t, jobs, f)
	require.Panics(t, func() {
		_, _ = jobs.ResolveCleanupQuarantine(ctx, f.a.ID, in, func(v backup.CleanupResolutionEvent) error {
			if v.Name == "accepted" {
				panic("owned interruption")
			}
			return nil
		})
	})
	bin, dsn := os.Getenv("POWERX_STORAGE_TEST_PG_BIN"), os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	require.True(t, filepath.IsAbs(bin))
	file := filepath.Join(t.TempDir(), "owned-quarantine.dump")
	run := func(name string, args ...string) {
		t.Helper()
		c, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		out, e := exec.CommandContext(c, filepath.Join(bin, name), args...).CombinedOutput()
		require.NoError(t, e, "%s: %s", name, out)
	}
	run("pg_dump", "--dbname="+dsn, "--format=custom", "--schema="+coremodel.PowerXSchema, "--file="+file)
	r, e := jobs.ResolveCleanupQuarantine(ctx, f.a.ID, in, func(backup.CleanupResolutionEvent) error { return nil })
	require.NoError(t, e)
	require.Equal(t, "quarantined", r.State)
	epoch, e := store.BeginChecked(ctx, uuid.NewString(), uuid.NewString(), func(records map[string]recovery.RetentionRecord) error {
		_, e := backup.CheckRetentionRecoveryCoverage(ctx, db, coremodel.PowerXSchema, records)
		return e
	})
	require.NoError(t, e)
	run("pg_restore", "--clean", "--if-exists", "--exit-on-error", "--no-owner", "--dbname="+dsn, file)
	assertRetainSources(t, db, f, false)
	inventory, e := jobs.QuarantineInventoryRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, "quarantined", inventory.Result)
	require.Equal(t, "not_quarantined", inventory.Current)
	require.Equal(t, "sql_replayed", inventory.HistoryStatus)
	diagnosis, e := jobs.InspectQuarantineArtifact(ctx, in.RequestID, inventory.Snapshot, func(backup.CleanupVerification) error { return nil })
	require.NoError(t, e)
	require.False(t, diagnosis.CurrentlyQuarantined)
	require.False(t, diagnosis.ValidBackupProof)
	assertRetainSources(t, db, f, false)

	got, e := jobs.CleanupQuarantineRequest(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, r, got)
	_, e = jobs.ResolveCleanupQuarantine(ctx, f.a.ID, in, func(backup.CleanupResolutionEvent) error {
		t.Fatal("historical operation reprotected current rows")
		return nil
	})
	require.NoError(t, e)
	assertRetainSources(t, db, f, false)
	state, e := s.ExecutionConfiguration(ctx)
	require.NoError(t, e)
	finishOwnedRecovery(t, s, apply, epoch.Epoch, state.DesiredRevision)
	fresh := quarantineInput(t, jobs, f)
	require.NotEqual(t, in.ReviewVersion, fresh.ReviewVersion)
	newReceipt, e := jobs.ResolveCleanupQuarantine(ctx, f.a.ID, fresh, func(backup.CleanupResolutionEvent) error { return nil })
	require.NoError(t, e)
	require.Equal(t, "quarantined", newReceipt.State)
	assertQuarantined(t, db, f)
	var old m.BackupCleanupResolution
	require.NoError(t, db.Where("request_id=?", in.RequestID).First(&old).Error)
	require.Equal(t, "checking", old.State, "the restored original SQL history remains unchanged")
}
