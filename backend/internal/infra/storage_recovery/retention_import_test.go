package storagerecovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRetentionImportGenuineV4UpgradePreservesHistory(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "old", true: "interrupted"}[mixed], func(t *testing.T) {
			s, dir, record := ledgerFixture(t)
			ctx := context.Background()
			_, e := s.ReserveRetention(ctx, record)
			require.NoError(t, e)
			require.NoError(t, s.locked(ctx, true, func(fd int) error {
				var b binding
				raw, e := read(fd, "storage-recovery-binding.json")
				if e != nil {
					return e
				}
				if e = strict(raw, &b); e != nil {
					return e
				}
				d, e := readRetentionLedger(fd, b)
				if e != nil {
					return e
				}
				if !mixed {
					d.Protocol = "powerx.backup-retention-ledger/v1"
				}
				if e = saveRetentionLedger(fd, d); e != nil {
					return e
				}
				b.Protocol = PreviousProtocol
				if mixed {
					b.Protocol = Protocol
				}
				epoch, e := s.readEpoch(fd)
				if e != nil {
					return e
				}
				epoch.Protocol = PreviousProtocol
				if e = write(fd, "storage-recovery-epoch.json", epoch); e != nil {
					return e
				}
				return write(fd, "storage-recovery-binding.json", b)
			}))
			_, e = Open(dir, s.installation)
			require.Error(t, e)
			s, e = UpgradeV1(dir, s.installation)
			require.NoError(t, e)
			after, e := s.Retention(ctx, record.RequestID)
			require.NoError(t, e)
			require.Equal(t, record, after)
			raw, e := os.ReadFile(filepath.Join(dir, retentionFile))
			require.NoError(t, e)
			var d retentionLedger
			require.NoError(t, json.Unmarshal(raw, &d))
			require.Equal(t, retentionProtocol, d.Protocol)
			require.NoError(t, os.Remove(filepath.Join(dir, retentionFile)))
			// A recorded ledger cannot be recreated even if an old marker reappears.
			raw, e = os.ReadFile(filepath.Join(dir, "storage-recovery-binding.json"))
			require.NoError(t, e)
			var b binding
			require.NoError(t, json.Unmarshal(raw, &b))
			b.Protocol = PreviousProtocol
			raw, e = json.Marshal(b)
			require.NoError(t, e)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "storage-recovery-binding.json"), raw, 0600))
			_, e = UpgradeV1(dir, s.installation)
			require.Error(t, e)
		})
	}
}

func TestRetentionImportAuditBoundAndCheckedRestoreNoAdvance(t *testing.T) {
	s, _, record := ledgerFixture(t)
	ctx := context.Background()
	inv, e := s.RetentionInventory(ctx)
	require.NoError(t, e)
	in := RetentionImport{RequestID: uuid.NewString(), Scope: record.Scope, ActorID: 2, InputDigest: strings.Repeat("a", 64), ExpectedSnapshot: strings.Repeat("b", 64), Generation: RetentionGeneration(record.Scope, inv.Epoch), Reason: "owned import", CreatedAt: time.Now().UTC().Truncate(time.Microsecond), State: "pending"}
	_, e = s.ReserveRetentionImport(ctx, in)
	require.NoError(t, e)
	_, e = s.ApplyRetentionImport(ctx, in, func(Epoch, map[string]RetentionRecord) (RetentionImportPlan, error) {
		return RetentionImportPlan{Records: []RetentionRecord{record}, SourceDigest: strings.Repeat("c", 64), Total: 1, Covered: 1, Unknown: 1}, nil
	})
	require.NoError(t, e)
	before, e := s.Read(ctx)
	require.NoError(t, e)
	_, e = s.BeginChecked(ctx, uuid.NewString(), uuid.NewString(), func(records map[string]RetentionRecord) error {
		require.Contains(t, records, record.RequestID)
		return ErrUnavailable
	})
	require.Error(t, e)
	after, e := s.Read(ctx)
	require.NoError(t, e)
	require.Equal(t, before, after)
	require.NoError(t, s.retention(ctx, true, func(fd int, d *retentionLedger) error {
		for len(d.Imports) < MaxRetentionImports {
			r := in
			r.RequestID = uuid.NewString()
			d.Imports[r.RequestID] = r
		}
		return saveRetentionLedger(fd, *d)
	}))
	in.RequestID = uuid.NewString()
	_, e = s.ReserveRetentionImport(ctx, in)
	require.ErrorIs(t, e, ErrRetentionFull)
	_, e = s.Retention(ctx, record.RequestID)
	require.NoError(t, e)
}

func TestCleanupResolutionUpgradeV5PreservesImportAudit(t *testing.T) {
	s, dir, r := ledgerFixture(t)
	ctx := context.Background()
	_, e := s.ReserveRetention(ctx, r)
	require.NoError(t, e)
	inv, e := s.RetentionInventory(ctx)
	require.NoError(t, e)
	in := RetentionImport{RequestID: uuid.NewString(), Scope: r.Scope, ActorID: 2, InputDigest: strings.Repeat("a", 64), ExpectedSnapshot: strings.Repeat("b", 64), Generation: RetentionGeneration(r.Scope, inv.Epoch), Reason: "preserve old import audit", CreatedAt: time.Now().UTC().Truncate(time.Microsecond), State: "pending"}
	_, e = s.ReserveRetentionImport(ctx, in)
	require.NoError(t, e)
	require.NoError(t, s.locked(ctx, true, func(fd int) error {
		var b binding
		raw, e := read(fd, "storage-recovery-binding.json")
		if e != nil {
			return e
		}
		if e = strict(raw, &b); e != nil {
			return e
		}
		d, e := readRetentionLedger(fd, b)
		if e != nil {
			return e
		}
		d.Protocol = "powerx.backup-retention-ledger/v2"
		if e = saveRetentionLedger(fd, d); e != nil {
			return e
		}
		epoch, e := s.readEpoch(fd)
		if e != nil {
			return e
		}
		b.Protocol = ImportProtocol
		epoch.Protocol = ImportProtocol
		if e = write(fd, "storage-recovery-binding.json", b); e != nil {
			return e
		}
		return write(fd, "storage-recovery-epoch.json", epoch)
	}))
	s, e = UpgradeV1(dir, s.installation)
	require.NoError(t, e)
	after, e := s.RetentionImport(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, in, after)
	before, e := s.Retention(ctx, r.RequestID)
	require.NoError(t, e)
	require.Equal(t, r, before)
	disposition := r
	disposition.RequestID = uuid.NewString()
	disposition.Operation = "retain_cleanup"
	disposition.HistoryDigest = strings.Repeat("e", 64)
	disposition.JobID = 1
	disposition.PolicyID = 1
	_, e = s.ReserveRetention(ctx, disposition)
	require.NoError(t, e)
	old, e := s.Retentions(ctx, r.Scope, r.ArtifactID, r.Token)
	require.NoError(t, e)
	require.Len(t, old, 1, "typed disposition cannot masquerade as original retention request")
	newRows, e := s.CleanupResolutions(ctx, r.Scope, r.ArtifactID, r.Token)
	require.NoError(t, e)
	require.Len(t, newRows, 1)
}

func TestCleanupQuarantineUpgradeV6PreservesTypedDispositionAndImportAudit(t *testing.T) {
	s, dir, r := ledgerFixture(t)
	ctx := context.Background()
	_, e := s.ReserveRetention(ctx, r)
	require.NoError(t, e)
	inv, e := s.RetentionInventory(ctx)
	require.NoError(t, e)
	in := RetentionImport{RequestID: uuid.NewString(), Scope: r.Scope, ActorID: 2, InputDigest: strings.Repeat("a", 64), ExpectedSnapshot: strings.Repeat("b", 64), Generation: RetentionGeneration(r.Scope, inv.Epoch), Reason: "preserve old import audit", CreatedAt: time.Now().UTC().Truncate(time.Microsecond), State: "pending"}
	_, e = s.ReserveRetentionImport(ctx, in)
	require.NoError(t, e)
	prior := r
	prior.RequestID = uuid.NewString()
	prior.Operation = "retain_cleanup"
	prior.HistoryDigest = strings.Repeat("e", 64)
	prior.JobID = 1
	prior.PolicyID = 1
	_, e = s.ReserveRetention(ctx, prior)
	require.NoError(t, e)
	require.NoError(t, s.locked(ctx, true, func(fd int) error {
		var b binding
		raw, e := read(fd, "storage-recovery-binding.json")
		if e != nil {
			return e
		}
		if e = strict(raw, &b); e != nil {
			return e
		}
		d, e := readRetentionLedger(fd, b)
		if e != nil {
			return e
		}
		d.Protocol = "powerx.backup-retention-ledger/v3"
		if e = saveRetentionLedger(fd, d); e != nil {
			return e
		}
		epoch, e := s.readEpoch(fd)
		if e != nil {
			return e
		}
		b.Protocol = ResolutionProtocol
		epoch.Protocol = ResolutionProtocol
		if e = write(fd, "storage-recovery-binding.json", b); e != nil {
			return e
		}
		return write(fd, "storage-recovery-epoch.json", epoch)
	}))
	s, e = UpgradeV1(dir, s.installation)
	require.NoError(t, e)
	after, e := s.RetentionImport(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, in, after)
	before, e := s.Retention(ctx, r.RequestID)
	require.NoError(t, e)
	require.Equal(t, r, before)
	got, e := s.Retention(ctx, prior.RequestID)
	require.NoError(t, e)
	require.Equal(t, prior, got)
	disposition := r
	disposition.RequestID = uuid.NewString()
	disposition.Operation = "quarantine_cleanup"
	disposition.ExpectedDiagnosis = "missing"
	disposition.DiagnosticDigest = strings.Repeat("f", 64)
	disposition.HistoryDigest = strings.Repeat("e", 64)
	disposition.JobID = 1
	disposition.PolicyID = 1
	_, e = s.ReserveRetention(ctx, disposition)
	require.NoError(t, e)
	old, e := s.Retentions(ctx, r.Scope, r.ArtifactID, r.Token)
	require.NoError(t, e)
	require.Len(t, old, 1, "typed disposition cannot masquerade as original retention request")
	newRows, e := s.CleanupResolutions(ctx, r.Scope, r.ArtifactID, r.Token)
	require.NoError(t, e)
	require.Len(t, newRows, 2)
}
