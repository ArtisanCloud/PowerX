package marketplace

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDistributionCredentialsNeverFollowRedirects(t *testing.T) {
	destinationCalls := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationCalls++
		t.Error("credentials followed redirect")
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer pxm_instance_test", r.Header.Get("Authorization"))
		require.Equal(t, Fingerprint("instance-a"), r.Header.Get("X-PowerX-Fingerprint"))
		http.Redirect(w, r, destination.URL, 307)
	}))
	defer source.Close()
	client := NewClient(source.URL, "", source.Client())
	client.SetInstance("pxm_instance_test", Fingerprint("instance-a"))
	err := client.Heartbeat(context.Background(), "1.0.0")
	require.Error(t, err)
	require.False(t, Retryable(err))
	require.Zero(t, destinationCalls)
}
func TestDistributionStoreLocksAndRejectsForeignOrCorruptState(t *testing.T) {
	dir := t.TempDir()
	a, err := OpenDistributionStore(dir, "https://market.invalid/api/v1", "instance-a")
	require.NoError(t, err)
	defer a.Close()
	_, err = OpenDistributionStore(dir, "https://market.invalid/api/v1", "instance-a")
	require.Error(t, err)
	instanceID := uuid.NewString()
	require.NoError(t, a.Update(func(st *DistributionState) error {
		st.InstanceID = instanceID
		st.InstanceToken = "pxm_instance_test"
		return nil
	}))
	b, err := OpenDistributionStore(dir, "https://market.invalid/api/v1", "instance-b")
	require.NoError(t, err)
	file := b.file
	require.NoError(t, b.Close())
	raw, err := os.ReadFile(a.file)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, raw, 0o600))
	_, err = OpenDistributionStore(dir, "https://market.invalid/api/v1", "instance-b")
	require.Error(t, err)
	copy := a.Snapshot()
	copy.InstanceID = "mutated"
	require.Equal(t, instanceID, a.Snapshot().InstanceID)
	foreign := filepath.Join(dir, "foreign")
	require.NoError(t, os.WriteFile(foreign, []byte("{}"), 0o600))
	require.NoError(t, a.Close())
	require.NoError(t, os.Remove(a.file))
	require.NoError(t, os.Symlink(foreign, a.file))
	_, err = OpenDistributionStore(dir, "https://market.invalid/api/v1", "instance-a")
	require.Error(t, err)
}

func TestDistributionStoreRejectsCorruptPendingCredential(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenDistributionStore(dir, "https://market.invalid/api/v1", "pending-test")
	require.NoError(t, err)
	require.NoError(t, store.Update(func(st *DistributionState) error { st.PendingInstanceToken = "pxm_instance_corrupt"; return nil }))
	file := store.file
	require.NoError(t, store.Close())
	before, err := os.ReadFile(file)
	require.NoError(t, err)
	_, err = OpenDistributionStore(dir, "https://market.invalid/api/v1", "pending-test")
	require.Error(t, err)
	after, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "must not replace corrupt identity with a new credential")
}
