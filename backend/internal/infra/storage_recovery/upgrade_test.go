package storagerecovery

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradeKeepsIndependentEpochAndResumesInterruptedWrite(t *testing.T) {
	for _, protocol := range []string{LegacyProtocol, IntermediateProtocol, OlderProtocol, PreviousProtocol, ImportProtocol, ResolutionProtocol, Protocol} {
		for _, mixed := range []bool{false, true} {
			t.Run(protocol+map[bool]string{false: "/complete", true: "/interrupted"}[mixed], func(t *testing.T) {
				dir := privateAnchorDirectory(t)
				id := uuid.NewString()
				s, e := Initialize(dir, id)
				require.NoError(t, e)
				before, e := s.Begin(context.Background(), uuid.NewString(), uuid.NewString())
				require.NoError(t, e)
				old := before
				old.Protocol = protocol
				raw, e := os.ReadFile(filepath.Join(dir, "storage-recovery-binding.json"))
				require.NoError(t, e)
				var b binding
				require.NoError(t, json.Unmarshal(raw, &b))
				b.Protocol = protocol
				if mixed {
					b.Protocol = Protocol
				}
				for name, value := range map[string]any{"storage-recovery-binding.json": b, "storage-recovery-epoch.json": old} {
					raw, _ := json.Marshal(value)
					require.NoError(t, os.WriteFile(filepath.Join(dir, name), raw, 0600))
				}
				_, e = Open(dir, id)
				if protocol != Protocol {
					require.Error(t, e)
				} else {
					require.NoError(t, e)
				}
				s, e = UpgradeV1(dir, id)
				require.NoError(t, e)
				after, e := s.Read(context.Background())
				require.NoError(t, e)
				require.Equal(t, before, after)
				_, e = UpgradeV1(dir, id)
				require.NoError(t, e, "repeated upgrade preserves the existing anchor/epoch")
				require.NoError(t, os.Remove(filepath.Join(dir, "storage-recovery-epoch.json")))
				_, e = UpgradeV1(dir, id)
				require.Error(t, e, "upgrade must not recreate missing proof")
			})
		}
	}
}
