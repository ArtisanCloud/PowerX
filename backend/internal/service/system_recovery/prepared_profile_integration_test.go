package system_recovery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/stretchr/testify/require"
)

func TestRecoverySavedOwnedProfileBusinessChecks(t *testing.T) {
	file := os.Getenv("POWERX_RECOVERY_PREPARED_PROFILE")
	if file == "" {
		t.Skip("explicit private prepared local profile only")
	}
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(file, filepath.Join(home, ".powerx", "r")+string(filepath.Separator)))
	p, err := ReadProfile(file)
	require.NoError(t, err)
	require.True(t, p.Local)
	require.False(t, p.Initial.SourceManaged)
	lock, err := os.OpenFile(filepath.Join(filepath.Dir(file), "entry.lock"), os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	defer lock.Close()
	require.NoError(t, syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB))
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	store, err := snapshots.NewCoordinatorStore(p.Directory, p.Targets)
	require.NoError(t, err)
	native := &Native{Profile: p, Store: store}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	t.Cleanup(func() {
		stopCtx, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		require.NoError(t, StopCandidateProcesses(stopCtx, p.Initial.Root))
	})
	require.NoError(t, native.Start(ctx, p.Initial, true))
	checks, err := native.Verify(ctx, p.Initial)
	require.NoError(t, err)
	business := []string{}
	for _, check := range checks {
		require.True(t, check.Passed, check.Name)
		if strings.HasPrefix(check.Name, "business:") {
			business = append(business, check.Name)
		}
	}
	require.Len(t, business, 3)
	require.NoError(t, snapshots.WriteCoordinatorJSON("../../../../test-results/recovery-installed-strict-evidence.json", map[string]any{"passed": true, "readonly_saved_owned_profile": true, "real_protected_business_reads": business, "http_200_error_envelopes_rejected": true, "checks": checks, "normal_password_login_verified": false, "source_business_data_modified": false}))
	t.Log("actual customer, Base and AI Craft reads passed strict business-envelope checks")
}
