package audit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditRetentionHandoverReachesTheActualWriter(t *testing.T) {
	for _, managed := range []bool{false, true} {
		t.Run(map[bool]string{false: "native_defaults", true: "lifecycle_owner"}[managed], func(t *testing.T) {
			s, err := NewFileSink(FileSinkOptions{Enable: true, Dir: t.TempDir(), RetentionManaged: managed, Compress: true, MaxSize: 12})
			require.NoError(t, err)
			require.NoError(t, s.ensureWriterLocked("2026-10-30"))
			t.Cleanup(func() { _ = s.logger.Close() })
			require.Equal(t, 12, s.logger.MaxSize)
			require.True(t, s.logger.Compress)
			if managed {
				require.Zero(t, s.logger.MaxAge, "constructor defaults must not restore destructive retention")
				require.Zero(t, s.logger.MaxBackups)
			} else {
				require.Equal(t, 30, s.logger.MaxAge)
				require.Equal(t, 30, s.logger.MaxBackups)
			}
		})
	}
}
