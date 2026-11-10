package storagegov

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func damageQuarantineFixture(t *testing.T, db *gorm.DB, f retainRecoveryArtifact, kind string) {
	t.Helper()
	path := f.a.StorageURI[len("file://"):]
	switch kind {
	case "missing":
		require.NoError(t, os.Remove(path))
	case "size_changed":
		require.NoError(t, os.WriteFile(path, []byte("short"), 0600))
	case "checksum_mismatch":
		data, e := os.ReadFile(path)
		require.NoError(t, e)
		data[0] ^= 1
		require.NoError(t, os.WriteFile(path, data, 0600))
	case "sidecars_missing":
		require.NoError(t, os.Remove(path+".json"))
	case "sidecar_mismatch":
		require.NoError(t, os.WriteFile(path+".sha256", []byte("not a checksum"), 0600))
	case "metadata_invalid":
		require.NoError(t, db.Model(&m.BackupArtifact{}).Where("id=?", f.a.ID).Update("checksum", "bad").Error)
	}
}
func quarantineInput(t *testing.T, jobs *backup.JobService, f retainRecoveryArtifact) backup.CleanupQuarantineInput {
	t.Helper()
	v, e := jobs.PreviewCleanupQuarantine(controlRoot(), f.a.ID, f.a.CleanupToken, func(backup.CleanupVerification) error { return nil })
	require.NoError(t, e)
	require.True(t, v.CanQuarantine, "%+v", v)
	return backup.CleanupQuarantineInput{CleanupResolutionInput: backup.CleanupResolutionInput{RequestID: uuid.NewString(), ExpectedToken: f.a.CleanupToken, ReviewVersion: v.ReviewVersion, HistoryDigest: v.HistoryDigest, Reason: "owned quarantine, no deletion or usable-backup claim", Confirm: true}, ExpectedDiagnosis: v.Diagnosis, DiagnosticDigest: v.DiagnosticDigest}
}
func assertQuarantined(t *testing.T, db *gorm.DB, f retainRecoveryArtifact) {
	t.Helper()
	var a m.BackupArtifact
	var j m.BackupJob
	require.NoError(t, db.First(&a, f.a.ID).Error)
	require.NoError(t, db.First(&j, f.j.ID).Error)
	require.Equal(t, "quarantined", a.CleanupState)
	require.Equal(t, f.a.CleanupToken, a.CleanupToken)
	require.False(t, j.Protected)
	require.Equal(t, m.BackupJobStatusSuccess, j.Status)
}
func TestCleanupQuarantineNativeKnownDamageNoDeletionOrValidBackupClaim(t *testing.T) {
	for _, kind := range []string{"missing", "size_changed", "checksum_mismatch", "sidecars_missing", "sidecar_mismatch", "metadata_invalid"} {
		t.Run(kind, func(t *testing.T) {
			_, jobs, db, store, _, _, f := resolutionSetup(t)
			damageQuarantineFixture(t, db, f, kind)
			var original m.BackupCleanupRetention
			require.NoError(t, db.Where("request_id=?", f.in.RequestID).First(&original).Error)
			path := f.a.StorageURI[len("file://"):]
			before, readErr := os.ReadFile(path)
			in := quarantineInput(t, jobs, f)
			require.Equal(t, kind, in.ExpectedDiagnosis)
			r, e := jobs.ResolveCleanupQuarantine(controlRoot(), f.a.ID, in, func(backup.CleanupResolutionEvent) error { return nil })
			require.NoError(t, e)
			require.Equal(t, "quarantined", r.State)
			require.Equal(t, "quarantine_cleanup", r.Operation)
			require.False(t, r.ValidBackupProof)
			require.True(t, r.IndependentComplete)
			assertQuarantined(t, db, f)
			var after m.BackupCleanupRetention
			require.NoError(t, db.Where("request_id=?", f.in.RequestID).First(&after).Error)
			require.Equal(t, original, after)
			got, e := os.ReadFile(path)
			if readErr != nil {
				require.True(t, os.IsNotExist(e))
			} else {
				require.NoError(t, e)
				require.Equal(t, before, got)
			}
			_, e = jobs.ResolveCleanupQuarantine(controlRoot(), f.a.ID, in, func(backup.CleanupResolutionEvent) error { t.Fatal("terminal replay did work"); return nil })
			require.NoError(t, e)
			_, _, e = jobs.OpenDownload(controlRoot(), f.j.ID, "owned")
			require.Equal(t, "BACKUP_ARTIFACT_QUARANTINED", dto.CodeOf(e))
			e = jobs.SetProtected(controlRoot(), f.j.ID, true, "owned", "owned")
			require.Equal(t, "BACKUP_ARTIFACT_QUARANTINED", dto.CodeOf(e))
			_, e = backup.NewRestoreDrillService(db).Trigger(controlRoot(), backup.TriggerRestoreDrillRequest{SourceJobID: f.j.ID, Operator: "owned"})
			require.Equal(t, "BACKUP_ARTIFACT_QUARANTINED", dto.CodeOf(e))
			copies := backup.NewRestoreCopyService(controlRoot(), db, backup.RestoreCopyOptions{Directory: t.TempDir()})
			_, e = copies.Plan(controlRoot(), backup.RestoreSource{SourceJobID: f.j.ID})
			copies.Close()
			require.Equal(t, "BACKUP_ARTIFACT_QUARANTINED", dto.CodeOf(e))

			_, n, e := jobs.ListJobs(controlRoot(), backup.ListJobOptions{View: "available", Page: 1, PageSize: 20})
			require.NoError(t, e)
			require.Zero(t, n)
			_, n, e = jobs.ListJobs(controlRoot(), backup.ListJobOptions{View: "history", Page: 1, PageSize: 20})
			require.NoError(t, e)
			require.Equal(t, int64(1), n)
			_, e = jobs.CleanupResolutionRequest(controlRoot(), in.RequestID)
			require.Equal(t, "BACKUP_RESOLUTION_REQUEST_NOT_FOUND", dto.CodeOf(e))
			rows, e := store.CleanupResolutions(controlRoot(), original.ScopeFingerprint, f.a.ID, f.a.CleanupToken)
			require.NoError(t, e)
			require.Len(t, rows, 1)
		})
	}
}
func TestCleanupQuarantineNativeFaultsPreserveUnknownAndCurrentIntent(t *testing.T) {
	for _, kind := range []string{"healthy", "appeared", "progress-change", "epoch", "accepted-loss", "rollback", "checkpoint", "policy-lock"} {
		t.Run(kind, func(t *testing.T) {
			_, jobs, db, store, dir, _, f := resolutionSetup(t)
			ctx := controlRoot()
			path := f.a.StorageURI[len("file://"):]
			if kind == "healthy" {
				v, e := jobs.PreviewCleanupQuarantine(ctx, f.a.ID, f.a.CleanupToken, func(backup.CleanupVerification) error { return nil })
				require.NoError(t, e)
				require.False(t, v.CanQuarantine)
				require.Equal(t, "verified", v.Diagnosis)
				return
			}
			damageQuarantineFixture(t, db, f, "missing")
			in := quarantineInput(t, jobs, f)
			if kind == "appeared" {
				require.NoError(t, os.WriteFile(path, []byte("new file arrived"), 0600))
			}
			if kind == "policy-lock" {
				unlock, e := lockOwnedBackupPolicy(db, f.j.PolicyID)
				require.NoError(t, e)
				defer unlock()
			}
			name := "owned_quarantine_fault"
			ledger := filepath.Join(dir, "storage-retention-ledger.json")
			saved := filepath.Join(dir, "owned-saved-ledger")
			faulted := false
			if kind == "rollback" || kind == "checkpoint" {
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
					v, ok := tx.Statement.Dest.(map[string]any)
					if tx.Statement.Table != "ops_backup_cleanup_resolutions" || !ok || v["state"] != "quarantined" {
						return
					}
					if kind == "rollback" {
						tx.AddError(errors.New("owned SQL rollback"))
					} else {
						require.NoError(t, os.Rename(ledger, saved))
						require.NoError(t, os.Mkdir(ledger, 0700))
						faulted = true
					}
				}))
				defer db.Callback().Update().Remove(name)
			}
			out, e := jobs.ResolveCleanupQuarantine(ctx, f.a.ID, in, func(v backup.CleanupResolutionEvent) error {
				switch {
				case kind == "accepted-loss" && v.Name == "accepted":
					return errors.New("owned disconnect")
				case kind == "epoch" && v.Name == "accepted":
					_, e := store.Begin(ctx, uuid.NewString(), uuid.NewString())
					require.NoError(t, e)
				case kind == "progress-change" && v.Name == "progress":
					require.NoError(t, os.WriteFile(path, []byte("appeared during evidence read"), 0600))
				}
				return nil
			})
			if faulted {
				require.NoError(t, os.Remove(ledger))
				require.NoError(t, os.Rename(saved, ledger))
			}
			switch kind {
			case "checkpoint":
				require.Error(t, e)
				assertQuarantined(t, db, f)
				out, e = jobs.ResolveCleanupQuarantine(ctx, f.a.ID, in, func(backup.CleanupResolutionEvent) error { t.Fatal("checkpoint re-executed SQL"); return nil })
				require.NoError(t, e)
				require.Equal(t, "quarantined", out.State)
			case "accepted-loss":
				require.Error(t, e)
				r, err := jobs.CleanupQuarantineRequest(ctx, in.RequestID)
				require.NoError(t, err)
				require.True(t, r.CanResume)
				out, e = jobs.ResolveCleanupQuarantine(ctx, f.a.ID, in, func(backup.CleanupResolutionEvent) error { return nil })
				require.NoError(t, e)
				require.Equal(t, "quarantined", out.State)
			case "epoch":
				require.Error(t, e)
				r, err := jobs.CleanupQuarantineRequest(ctx, in.RequestID)
				require.NoError(t, err)
				require.True(t, r.RecoveryBlocked)
				assertRetainSources(t, db, f, false)
			case "policy-lock":
				require.Error(t, e)
				assertRetainSources(t, db, f, false)
			default:
				require.NoError(t, e)
				require.Equal(t, "rejected", out.State)
				assertRetainSources(t, db, f, false)
			}
		})
	}
}
func TestCleanupQuarantineNativeConcurrentSingleWinner(t *testing.T) {
	_, jobs, db, _, _, _, f := resolutionSetup(t)
	damageQuarantineFixture(t, db, f, "sidecars_missing")
	in := quarantineInput(t, jobs, f)
	other := in
	other.RequestID = uuid.NewString()
	results := make(chan backup.CleanupResolutionReceipt, 2)
	var wg sync.WaitGroup
	for _, v := range []backup.CleanupQuarantineInput{in, other} {
		wg.Add(1)
		go func(input backup.CleanupQuarantineInput) {
			defer wg.Done()
			r, _ := jobs.ResolveCleanupQuarantine(controlRoot(), f.a.ID, input, func(backup.CleanupResolutionEvent) error { return nil })
			results <- r
		}(v)
	}
	wg.Wait()
	close(results)
	n := 0
	for r := range results {
		if r.State == "quarantined" {
			n++
		}
	}
	require.Equal(t, 1, n)
	assertQuarantined(t, db, f)
}

func TestCleanupQuarantineNativeNewColumnReadinessIsOptionalForAdmission(t *testing.T) {
	s, jobs, db, _, _, _, f := resolutionSetup(t)
	before := dto.CodeOf(s.CheckDatabaseAdmission(controlRoot()))
	require.NoError(t, db.Migrator().DropColumn(&m.BackupCleanupResolution{}, "diagnostic_digest"))
	_, e := jobs.PreviewCleanupQuarantine(controlRoot(), f.a.ID, f.a.CleanupToken, func(backup.CleanupVerification) error { return nil })
	require.Equal(t, "BACKUP_RESOLUTION_SCHEMA_NOT_READY", dto.CodeOf(e))
	require.Equal(t, before, dto.CodeOf(s.CheckDatabaseAdmission(controlRoot())))
}
