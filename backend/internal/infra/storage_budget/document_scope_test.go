package storagebudget

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstallationDocumentsAreIsolatedAndLegacyOwnerSurvivesStartupOrder(t *testing.T) {
	ctx := context.Background()
	a := fixture(t, "a", "", time.Now)
	b := fixture(t, "b", a.cfg.Directory, time.Now)
	sum := sha256.Sum256([]byte("host\x00disk\x00a"))
	scope := hex.EncodeToString(sum[:])
	binding, _ := json.Marshal(map[string]any{"version": 1, "scope": scope, "revision": 1})
	require.NoError(t, os.WriteFile(filepath.Join(a.cfg.Directory, "execution-control-binding.json"), binding, 0600))
	// 第二实例先启动也不能抢占旧绑定。
	raw, e := b.Document(ctx, "execution-control-binding", nil)
	require.NoError(t, e)
	require.Empty(t, raw)
	raw, e = a.Document(ctx, "execution-control-binding", nil)
	require.NoError(t, e)
	require.JSONEq(t, string(binding), string(raw))
	for _, item := range []struct {
		c     *Coordinator
		value string
	}{{a, "a"}, {b, "b"}} {
		_, e = item.c.Document(ctx, "table-monitoring", func([]byte) ([]byte, error) { return []byte(`{"owner":"` + item.value + `"}`), nil })
		require.NoError(t, e)
	}
	raw, e = a.Document(ctx, "table-monitoring", nil)
	require.NoError(t, e)
	require.JSONEq(t, `{"owner":"a"}`, string(raw))
	raw, e = b.Document(ctx, "table-monitoring", nil)
	require.NoError(t, e)
	require.JSONEq(t, `{"owner":"b"}`, string(raw))
	require.NoError(t, os.Remove(filepath.Join(a.cfg.Directory, "document-owner.json")))
	raw, e = b.Document(ctx, "table-monitoring", nil)
	require.NoError(t, e)
	require.JSONEq(t, `{"owner":"b"}`, string(raw))
	require.NoError(t, os.Remove(filepath.Join(a.cfg.Directory, "document-owner.json")))
	require.NoError(t, os.Remove(filepath.Join(a.cfg.Directory, "execution-control-binding.json")))
	_, e = b.Document(ctx, "table-monitoring", nil)
	require.Error(t, e, "missing attribution cannot silently change a scoped document back to legacy")
}
