package storagegov

import (
	"errors"
	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func inventoryOptions() backup.QuarantineInventoryOptions {
	return backup.QuarantineInventoryOptions{Page: 1, PageSize: 20, Current: "all", Result: "all", Diagnosis: "all", Sort: "time", Direction: "desc"}
}
func TestQuarantineInventoryNativeReadOnlyKnownAndHealthyFiles(t *testing.T) {
	_, jobs, db, store, _, _, f := resolutionSetup(t)
	damageQuarantineFixture(t, db, f, "missing")
	in := quarantineInput(t, jobs, f)
	_, e := jobs.ResolveCleanupQuarantine(controlRoot(), f.a.ID, in, func(backup.CleanupResolutionEvent) error { return nil })
	require.NoError(t, e)
	before, e := store.RetentionInventory(controlRoot())
	require.NoError(t, e)
	page, e := jobs.QuarantineInventory(controlRoot(), inventoryOptions())
	require.NoError(t, e)
	require.Equal(t, 1, page.Total)
	require.Zero(t, page.UnattributedQuarantinedArtifacts)
	r := page.Items[0]
	require.Equal(t, "covered", r.HistoryStatus)
	require.Equal(t, "quarantined", r.Current)
	out, e := jobs.InspectQuarantineArtifact(controlRoot(), r.RequestID, r.Snapshot, func(backup.CleanupVerification) error { return nil })
	require.NoError(t, e)
	require.Equal(t, "missing", out.Verification.State)
	require.False(t, out.ValidBackupProof)
	require.True(t, out.CurrentlyQuarantined)
	path := f.a.StorageURI[len("file://"):]
	require.NoError(t, os.WriteFile(path, []byte("owned recovery binding artifact\n"), 0600))
	out, e = jobs.InspectQuarantineArtifact(controlRoot(), r.RequestID, r.Snapshot, func(backup.CleanupVerification) error { return nil })
	require.NoError(t, e)
	require.Equal(t, "verified", out.Verification.State)
	require.True(t, out.Verification.ContentVerified)
	require.False(t, out.ValidBackupProof)
	require.True(t, out.QuarantineUnchanged)
	after, e := store.RetentionInventory(controlRoot())
	require.NoError(t, e)
	require.Equal(t, before, after)
	assertQuarantined(t, db, f)
	_, e = jobs.InspectQuarantineArtifact(controlRoot(), r.RequestID, r.Snapshot, func(backup.CleanupVerification) error { return errors.New("owned disconnect") })
	require.Error(t, e)
	assertQuarantined(t, db, f)
}
func TestQuarantineInventoryNativeSourceFailuresAndExactIdentity(t *testing.T) {
	for _, kind := range []string{"sql-missing", "conflict", "missing-artifact", "soft-deleted", "orphan-job", "token-changed", "unattributed", "source-unavailable", "anchor-missing", "limit"} {
		t.Run(kind, func(t *testing.T) {
			_, jobs, db, _, dir, _, f := resolutionSetup(t)
			damageQuarantineFixture(t, db, f, "missing")
			in := quarantineInput(t, jobs, f)
			_, e := jobs.ResolveCleanupQuarantine(controlRoot(), f.a.ID, in, func(backup.CleanupResolutionEvent) error { return nil })
			require.NoError(t, e)
			switch kind {
			case "sql-missing":
				require.NoError(t, db.Unscoped().Where("request_id=?", in.RequestID).Delete(&m.BackupCleanupResolution{}).Error)
			case "conflict":
				require.NoError(t, db.Model(&m.BackupCleanupResolution{}).Where("request_id=?", in.RequestID).Update("reason", "changed SQL reason").Error)
			case "missing-artifact":
				require.NoError(t, db.Unscoped().Delete(&m.BackupArtifact{}, f.a.ID).Error)
			case "soft-deleted":
				require.NoError(t, db.Delete(&m.BackupArtifact{}, f.a.ID).Error)
			case "orphan-job":
				require.NoError(t, db.Unscoped().Delete(&m.BackupJob{}, f.j.ID).Error)
			case "token-changed":
				require.NoError(t, db.Model(&m.BackupArtifact{}).Where("id=?", f.a.ID).Update("cleanup_token", uuid.NewString()).Error)
			case "unattributed":
				require.NoError(t, db.Create(&m.BackupArtifact{JobID: f.j.ID, CleanupState: "quarantined", CleanupToken: uuid.NewString()}).Error)
			case "source-unavailable":
				require.NoError(t, db.Migrator().DropColumn(&m.BackupCleanupResolution{}, "operation"))
			case "anchor-missing":
				require.NoError(t, os.Remove(filepath.Join(dir, "storage-retention-ledger.json")))
			case "limit":
				var original m.BackupCleanupResolution
				require.NoError(t, db.Where("request_id=?", in.RequestID).First(&original).Error)
				rows := make([]m.BackupCleanupResolution, 4097)
				for i := range rows {
					r := original
					r.ID = 0
					r.UUID = uuid.Nil
					r.RequestID = uuid.New()
					rows[i] = r
				}
				require.NoError(t, db.CreateInBatches(&rows, 128).Error)
			}
			page, e := jobs.QuarantineInventory(controlRoot(), inventoryOptions())
			if kind == "source-unavailable" || kind == "anchor-missing" || kind == "limit" {
				require.Error(t, e)
				return
			}
			require.NoError(t, e)
			require.Equal(t, 1, page.Total)
			r := page.Items[0]
			switch kind {
			case "sql-missing":
				require.Equal(t, "sql_missing", r.HistoryStatus)
			case "conflict":
				require.Equal(t, "conflict", r.HistoryStatus)
				require.False(t, r.CanInspect)
			case "missing-artifact":
				require.Equal(t, "missing", r.Current)
			case "soft-deleted", "orphan-job":
				require.Equal(t, "unavailable", r.Current)
			case "token-changed":
				require.Equal(t, "identity_changed", r.Current)
				require.False(t, r.CanInspect)
			case "unattributed":
				require.Equal(t, 1, page.UnattributedQuarantinedArtifacts)
			}
		})
	}
}
func TestQuarantineInventoryNativeMetadataEpochAndPolicyChangesStopInspection(t *testing.T) {
	for _, kind := range []string{"metadata", "epoch", "policy"} {
		t.Run(kind, func(t *testing.T) {
			_, jobs, db, store, _, _, f := resolutionSetup(t)
			damageQuarantineFixture(t, db, f, "missing")
			in := quarantineInput(t, jobs, f)
			_, e := jobs.ResolveCleanupQuarantine(controlRoot(), f.a.ID, in, func(backup.CleanupResolutionEvent) error { return nil })
			require.NoError(t, e)
			r, e := jobs.QuarantineInventoryRequest(controlRoot(), in.RequestID)
			require.NoError(t, e)
			if kind == "policy" {
				unlock, e := lockOwnedBackupPolicy(db, f.j.PolicyID)
				require.NoError(t, e)
				defer unlock()
			}
			_, e = jobs.InspectQuarantineArtifact(controlRoot(), r.RequestID, r.Snapshot, func(backup.CleanupVerification) error {
				if kind == "metadata" {
					require.NoError(t, db.Model(&m.BackupArtifact{}).Where("id=?", f.a.ID).Update("size_bytes", 999).Error)
				} else if kind == "epoch" {
					_, e := store.Begin(controlRoot(), uuid.NewString(), uuid.NewString())
					require.NoError(t, e)
				}
				return nil
			})
			require.Error(t, e)
			assertQuarantined(t, db, f)
		})
	}
}
