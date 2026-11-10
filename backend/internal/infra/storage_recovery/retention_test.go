package storagerecovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func ledgerFixture(t *testing.T) (*Store, string, RetentionRecord) {
	t.Helper()
	dir := privateAnchorDirectory(t)
	s, e := Initialize(dir, uuid.NewString())
	require.NoError(t, e)
	r := RetentionRecord{RequestID: uuid.NewString(), Scope: strings.Repeat("a", 64), InputDigest: strings.Repeat("b", 64), ReviewVersion: strings.Repeat("c", 64), Generation: strings.Repeat("d", 64), ArtifactID: 1, ActorID: 2, Token: uuid.NewString(), Reason: "owned retention", CreatedAt: time.Now().UTC().Truncate(time.Microsecond), State: "checking"}
	return s, dir, r
}
func TestRetentionLedgerImmutableIdentityTerminalAndRecovery(t *testing.T) {
	s, dir, in := ledgerFixture(t)
	ctx := context.Background()
	_, e := s.ReserveRetention(ctx, in)
	require.NoError(t, e)
	changed := in
	changed.InputDigest = strings.Repeat("e", 64)
	_, e = s.ReserveRetention(ctx, changed)
	require.ErrorIs(t, e, ErrRetentionConflict)
	now := time.Now().UTC().Truncate(time.Microsecond)
	done := in
	done.State = "retained"
	done.CompletedAt = &now
	require.NoError(t, s.CompleteRetention(ctx, done))
	require.NoError(t, s.CompleteRetention(ctx, done))
	other := done
	other.State = "rejected"
	other.Problem = "BACKUP_RETAIN_REJECTED"
	require.ErrorIs(t, s.CompleteRetention(ctx, other), ErrRetentionConflict)
	_, e = s.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, e)
	s, e = Open(dir, s.installation)
	require.NoError(t, e)
	r, e := s.Retention(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, done, r)
	rows, e := s.Retentions(ctx, in.Scope, in.ArtifactID, in.Token)
	require.NoError(t, e)
	require.Len(t, rows, 1)
	rows, e = s.Retentions(ctx, strings.Repeat("f", 64), in.ArtifactID, in.Token)
	require.NoError(t, e)
	require.Empty(t, rows)
	// Epoch loss blocks writes but does not erase independent history.
	require.NoError(t, os.Remove(filepath.Join(dir, "storage-recovery-epoch.json")))
	r, e = s.Retention(ctx, in.RequestID)
	require.NoError(t, e)
	require.Equal(t, done, r)
	_, e = s.ReserveRetention(ctx, in)
	require.Error(t, e)
}
func TestRetentionLedgerMissingCorruptUnsafeAndCapacityFailClosed(t *testing.T) {
	for _, kind := range []string{"missing", "corrupt", "mode", "symlink", "hardlink", "foreign", "full"} {
		t.Run(kind, func(t *testing.T) {
			s, dir, in := ledgerFixture(t)
			ctx := context.Background()
			_, e := s.ReserveRetention(ctx, in)
			require.NoError(t, e)
			path := filepath.Join(dir, retentionFile)
			switch kind {
			case "missing":
				require.NoError(t, os.Remove(path))
			case "corrupt":
				require.NoError(t, os.WriteFile(path, []byte(`{"protocol":"bad"}`), 0600))
			case "mode":
				require.NoError(t, os.Chmod(path, 0644))
			case "symlink":
				raw, e := os.ReadFile(path)
				require.NoError(t, e)
				target := filepath.Join(dir, "other")
				require.NoError(t, os.WriteFile(target, raw, 0600))
				require.NoError(t, os.Remove(path))
				require.NoError(t, os.Symlink(target, path))
			case "hardlink":
				require.NoError(t, os.Link(path, filepath.Join(dir, "other")))
			case "foreign", "full":
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
					if kind == "foreign" {
						d.Installation = uuid.NewString()
					} else {
						for len(d.Records) < MaxRetentionRecords {
							r := in
							r.RequestID = uuid.NewString()
							d.Records[r.RequestID] = r
						}
					}
					return saveRetentionLedger(fd, d)
				}))
			}
			candidate := in
			candidate.RequestID = uuid.NewString()
			_, e = s.ReserveRetention(ctx, candidate)
			if kind == "full" {
				require.ErrorIs(t, e, ErrRetentionFull)
				_, e = s.Retention(ctx, in.RequestID)
				require.NoError(t, e)
			} else {
				require.Error(t, e)
				_, e = s.Begin(ctx, uuid.NewString(), uuid.NewString())
				require.Error(t, e, "managed SQL restore must not discard unreadable UUID history")
				_, e = UpgradeV1(dir, s.installation)
				require.Error(t, e, "recorded v4 ledger must never be rebuilt")
			}
		})
	}
}

func TestRetentionLedgerGenuineLegacyUpgradeCreatesOnlyFirstLedger(t *testing.T) {
	for _, protocol := range []string{LegacyProtocol, IntermediateProtocol, OlderProtocol} {
		t.Run(protocol, func(t *testing.T) {
			s, dir, _ := ledgerFixture(t)
			epoch, e := s.Read(context.Background())
			require.NoError(t, e)
			b := binding{Protocol: protocol, Installation: s.installation, AnchorID: epoch.AnchorID}
			epoch.Protocol = protocol
			for name, v := range map[string]any{"storage-recovery-binding.json": b, "storage-recovery-epoch.json": epoch} {
				raw, e := json.Marshal(v)
				require.NoError(t, e)
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), raw, 0600))
			}
			require.NoError(t, os.Remove(filepath.Join(dir, retentionFile)))
			upgraded, e := UpgradeV1(dir, s.installation)
			require.NoError(t, e)
			require.NoError(t, upgraded.RetentionReady(context.Background()))
			require.NoError(t, os.Remove(filepath.Join(dir, retentionFile)))
			_, e = UpgradeV1(dir, s.installation)
			require.Error(t, e, "v4 binding cannot reinitialize lost history")
		})
	}
}
func TestRetentionLedgerConcurrentReservationKeepsOneIdentity(t *testing.T) {
	s, _, in := ledgerFixture(t)
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, e := s.ReserveRetention(context.Background(), in); errs <- e }()
	}
	close(start)
	wg.Wait()
	close(errs)
	succeeded := 0
	for e := range errs {
		if e == nil {
			succeeded++
		} else {
			require.ErrorIs(t, e, ErrBusy)
		}
	}
	require.Positive(t, succeeded)
	r, e := s.Retention(context.Background(), in.RequestID)
	require.NoError(t, e)
	require.Equal(t, in, r)
}
func TestRetentionLedgerInterruptedUpgradeNeverDropsReservations(t *testing.T) {
	s, dir, in := ledgerFixture(t)
	_, e := s.ReserveRetention(context.Background(), in)
	require.NoError(t, e)
	// Simulate v4 binding already committed but old epoch protocol still on disk.
	path := filepath.Join(dir, "storage-recovery-epoch.json")
	raw, e := os.ReadFile(path)
	require.NoError(t, e)
	var epoch Epoch
	require.NoError(t, json.Unmarshal(raw, &epoch))
	epoch.Protocol = PreviousProtocol
	raw, e = json.Marshal(epoch)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(path, raw, 0600))
	s, e = UpgradeV1(dir, s.installation)
	require.NoError(t, e)
	r, e := s.Retention(context.Background(), in.RequestID)
	require.NoError(t, e)
	require.Equal(t, in, r)
}
