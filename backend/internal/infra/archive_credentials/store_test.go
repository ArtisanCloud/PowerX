package archivecredentials

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func ownedStore(t *testing.T) (*Store, Config, Keyring) {
	t.Helper()
	dir, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	secrets := filepath.Join(dir, "sealed")
	require.NoError(t, os.Mkdir(secrets, 0700))
	key := make([]byte, 32)
	_, e = rand.Read(key)
	require.NoError(t, e)
	c := Config{Directory: secrets, KeyringFile: filepath.Join(dir, "external-keyring.json"), StoreID: "owned-test"}
	k := Keyring{Version: 1, StoreID: c.StoreID, ActiveKeyID: "k1", Keys: map[string]string{"k1": base64.StdEncoding.EncodeToString(key)}}
	writeKeyring(t, c, k)
	s, e := New(c, "owned-install")
	require.NoError(t, e)
	return s, c, k
}
func writeKeyring(t *testing.T, c Config, k Keyring) {
	t.Helper()
	raw, e := json.Marshal(k)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(c.KeyringFile, raw, 0600))
}
func savedInput() Input {
	return Input{Action: "save", RequestID: uuid.NewString(), Ref: "test", Actor: 11, Value: Value{AccessKey: "fixture-private-access", SecretKey: "fixture-private-secret"}}
}
func TestEncryptedStoreCASReceiptRevocationAndNoPlaintext(t *testing.T) {
	s, c, _ := ownedStore(t)
	ctx := context.Background()
	in := savedInput()
	out, e := s.Mutate(ctx, in)
	require.NoError(t, e)
	require.EqualValues(t, 1, out.Revision)
	require.EqualValues(t, 1, out.StoreRevision)
	raw, e := os.ReadFile(filepath.Join(c.Directory, ledgerName))
	require.NoError(t, e)
	require.NotContains(t, string(raw), in.Value.AccessKey)
	require.NotContains(t, string(raw), in.Value.SecretKey)
	replay, e := s.Mutate(ctx, in)
	require.NoError(t, e)
	require.Equal(t, out, replay)
	different := in
	different.Value.SecretKey = "different"
	_, e = s.Mutate(ctx, different)
	require.ErrorIs(t, e, ErrRequestConflict)
	_, e = s.Mutate(ctx, savedInput())
	require.ErrorIs(t, e, ErrConflict)
	value, view, _, e := s.Get(ctx, "test")
	require.NoError(t, e)
	require.Equal(t, in.Value.SecretKey, value.SecretKey)
	require.EqualValues(t, 1, view.Revision)
	receipt, e := s.Request(ctx, in.RequestID, 11)
	require.NoError(t, e)
	require.Equal(t, out, receipt)
	_, e = s.Request(ctx, in.RequestID, 12)
	require.Error(t, e)
	revoked, e := s.Mutate(ctx, Input{Action: "revoke", Ref: "test", Actor: 11, ExpectedRevision: 1, RequestID: uuid.NewString()})
	require.NoError(t, e)
	require.EqualValues(t, 2, revoked.Revision)
	_, _, _, e = s.Get(ctx, "test")
	require.ErrorIs(t, e, ErrRevoked)
}
func TestKeyRotationMissingKeyTamperAndIndependentRecovery(t *testing.T) {
	s, c, k := ownedStore(t)
	ctx := context.Background()
	_, e := s.Mutate(ctx, savedInput())
	require.NoError(t, e)
	_, _, oldID, e := s.Get(ctx, "test")
	require.NoError(t, e)
	k.ActiveKeyID = "k2"
	key := make([]byte, 32)
	_, e = rand.Read(key)
	require.NoError(t, e)
	k.Keys["k2"] = base64.StdEncoding.EncodeToString(key)
	writeKeyring(t, c, k)
	require.True(t, s.Snapshot(ctx).RotationRequired)
	_, _, newID, e := s.Get(ctx, "test")
	require.NoError(t, e)
	require.NotEqual(t, oldID, newID)
	in := savedInput()
	in.Ref = "new"
	_, e = s.Mutate(ctx, in)
	require.ErrorIs(t, e, ErrRotationRequired)
	_, e = s.Mutate(ctx, Input{Action: "rewrap", Actor: 11, ExpectedRevision: 1, RequestID: uuid.NewString()})
	require.NoError(t, e)
	delete(k.Keys, "k1")
	writeKeyring(t, c, k)
	require.True(t, s.Snapshot(ctx).Ready)
	require.False(t, s.Snapshot(ctx).RotationRequired)
	other, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	sealed, e := os.ReadFile(filepath.Join(c.Directory, ledgerName))
	require.NoError(t, e)
	require.NoError(t, os.Chmod(other, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(other, ledgerName), sealed, 0600))
	restored, e := New(Config{Directory: other, StoreID: c.StoreID, KeyringFile: c.KeyringFile}, "owned-install")
	require.NoError(t, e)
	value, _, _, e := restored.Get(ctx, "test")
	require.NoError(t, e)
	require.Equal(t, "fixture-private-secret", value.SecretKey)
	wrongScope, e := New(c, "other-install")
	require.NoError(t, e)
	require.False(t, wrongScope.Snapshot(ctx).Ready)
	sealed[len(sealed)/2] ^= 1
	require.NoError(t, os.WriteFile(filepath.Join(c.Directory, ledgerName), sealed, 0600))
	require.False(t, s.Snapshot(ctx).Ready)
	require.NoError(t, os.Remove(c.KeyringFile))
	require.False(t, restored.Snapshot(ctx).Ready)
	_, _, _, e = restored.Get(ctx, "test")
	require.Error(t, e)
}
func TestConcurrentCASAndUnsafeFilesAreRejected(t *testing.T) {
	s, c, _ := ownedStore(t)
	ctx := context.Background()
	in := savedInput()
	_, e := s.Mutate(ctx, in)
	require.NoError(t, e)
	var group sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			next := savedInput()
			next.ExpectedRevision = 1
			_, err := s.Mutate(ctx, next)
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, ErrConflict)
		}
	}
	require.Equal(t, 1, success)
	require.NoError(t, os.Chmod(c.KeyringFile, 0644))
	require.False(t, s.Snapshot(ctx).Ready)
	require.NoError(t, os.Chmod(c.KeyringFile, 0600))
	copy := filepath.Join(filepath.Dir(c.Directory), "hard-link.json")
	require.NoError(t, os.Link(c.KeyringFile, copy))
	require.False(t, s.Snapshot(ctx).Ready)
	require.NoError(t, os.Remove(copy))
	require.NoError(t, os.Remove(filepath.Join(c.Directory, ledgerName)))
	require.NoError(t, os.Symlink(c.KeyringFile, filepath.Join(c.Directory, ledgerName)))
	require.False(t, s.Snapshot(ctx).Ready)
}
