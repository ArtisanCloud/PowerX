package snapshot_ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type changingBackend struct {
	*fixtureBackend
	attempts  atomic.Int32
	transient int32
	permanent bool
}

func (b *changingBackend) Capture(ctx context.Context, dest string) (string, error) {
	n := b.attempts.Add(1)
	if b.permanent {
		return "", errors.New("permission denied")
	}
	if n <= b.transient {
		if e := os.WriteFile(filepath.Join(dest, "partial.tmp"), []byte("incomplete old point"), 0600); e != nil {
			return "", e
		}
		return "", archiveops.ErrSourceChanged
	}
	return b.fixtureBackend.Capture(ctx, dest)
}
func TestOnlineCaptureRetriesSameJobWithoutMixingPoints(t *testing.T) {
	s, b, _ := fixtureService(t)
	changing := &changingBackend{fixtureBackend: b, transient: 1}
	s.opts.Backend = changing
	s.captureRetryDelays = []time.Duration{20 * time.Millisecond}
	key := uuid.NewString()
	j, e := s.Start(rootContext(), key)
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		row, err := s.Get(rootContext(), j.ID)
		j = row
		return err == nil && row.Status == "ready"
	}, 5*time.Second, time.Millisecond)
	require.Equal(t, 2, j.Attempt)
	require.Nil(t, j.RetryAt)
	require.Empty(t, j.Error)
	_, e = os.Stat(filepath.Join(s.data(j.ID), "partial.tmp"))
	require.True(t, os.IsNotExist(e))
	replay, e := s.Start(rootContext(), key)
	require.NoError(t, e)
	require.Equal(t, j.ID, replay.ID)
	require.Equal(t, int32(2), changing.attempts.Load())
}
func TestOnlineCaptureRetryBoundAndPermanentErrors(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		t.Run(map[bool]string{false: "changed", true: "permanent"}[permanent], func(t *testing.T) {
			s, b, _ := fixtureService(t)
			changing := &changingBackend{fixtureBackend: b, transient: 10, permanent: permanent}
			s.opts.Backend = changing
			s.captureRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
			j, e := s.Start(rootContext(), uuid.NewString())
			require.NoError(t, e)
			require.Eventually(t, func() bool {
				row, err := s.Get(rootContext(), j.ID)
				j = row
				return err == nil && row.Status == "failed"
			}, 5*time.Second, time.Millisecond)
			expected := int32(3)
			if permanent {
				expected = 1
			}
			require.Equal(t, expected, changing.attempts.Load())
			require.Nil(t, j.RetryAt)
			_, e = s.GetRestore(rootContext(), j.ID)
			require.Error(t, e)
			_, e = os.Stat(s.data(j.ID))
			require.True(t, os.IsNotExist(e))
		})
	}
}
func TestOnlineCaptureBackoffIsCancelledAndPublished(t *testing.T) {
	s, b, _ := fixtureService(t)
	changing := &changingBackend{fixtureBackend: b, transient: 10}
	s.opts.Backend = changing
	s.captureRetryDelays = []time.Duration{time.Hour}
	j, e := s.Start(rootContext(), uuid.NewString())
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		row, err := s.Get(rootContext(), j.ID)
		return err == nil && row.Phase == "retry_wait" && row.RetryAt != nil && row.ErrorCode == "SNAPSHOT_SOURCE_CHANGED"
	}, 5*time.Second, time.Millisecond)
	s.Close()
	row, e := s.snapshot(j.ID)
	require.NoError(t, e)
	require.Equal(t, "failed", row.Status)
	require.Nil(t, row.RetryAt)
}
