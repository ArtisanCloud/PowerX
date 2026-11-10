//go:build darwin

package storagebudget

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

// 显式本机只读验收：真实配置仅作为读源，换绑只发生在自有临时副本。
func TestVolumeRebindNativeReadOnlySource(t *testing.T) {
	source := os.Getenv("POWERX_STORAGE_REBIND_READONLY_SOURCE")
	if source == "" {
		t.Skip("opt-in read-only source; only temporary copy is mutated")
	}
	require.True(t, filepath.IsAbs(source))
	file := filepath.Join(source, "budget.json")
	before, e := os.ReadFile(file)
	require.NoError(t, e)
	var original ledger
	require.NoError(t, json.Unmarshal(before, &original))
	require.True(t, stableFilesystemMatches(source, original.FilesystemID))
	require.Empty(t, original.Reservations)
	root := t.TempDir()
	state := filepath.Join(root, "state")
	require.NoError(t, os.Mkdir(state, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(state, "budget.json"), before, 0600))
	for _, name := range []string{"lifecycle.json", "database-retention.json", "system-table-retention.json", "ownership.json"} {
		raw, e := os.ReadFile(filepath.Join(source, name))
		if os.IsNotExist(e) {
			continue
		}
		require.NoError(t, e)
		require.NoError(t, os.WriteFile(filepath.Join(state, name), raw, 0600))
	}
	c, e := New(Config{Directory: state, FilesystemPath: root, HostID: original.HostID, FilesystemID: original.FilesystemID, InstanceID: "owned-rebind-check"})
	require.NoError(t, e)
	_, e = c.Status(context.Background())
	require.NoError(t, e)
	raw, e := os.ReadFile(filepath.Join(state, "budget.json"))
	require.NoError(t, e)
	var copied ledger
	require.NoError(t, json.Unmarshal(raw, &copied))
	require.Equal(t, c.device, copied.Device)
	require.Equal(t, original.Policy, copied.Policy)
	require.Len(t, copied.DeviceRebindings, len(original.DeviceRebindings)+1)
	after, e := os.ReadFile(file)
	require.NoError(t, e)
	require.Equal(t, sha256.Sum256(before), sha256.Sum256(after))
	t.Log("actual APFS UUID verified; temporary ledger rebound; original ledger unchanged")
}

func TestVolumeIdentityParserRequiresUniqueRootUUID(t *testing.T) {
	id := "96F39FA6-D058-4DD9-9F01-145CC4E87D43"
	raw := []byte(`<plist><dict><key>VolumeUUID</key><string>` + id + `</string></dict></plist>`)
	v, ok := parseVolumeUUID(raw)
	require.True(t, ok)
	require.Equal(t, uuid.MustParse(id), v)
	for _, raw := range []string{`<plist><dict><key>VolumeUUID</key><integer>42</integer></dict></plist>`, `<plist><dict><key>nested</key><dict><key>VolumeUUID</key><string>` + id + `</string></dict></dict></plist>`, `<plist><dict><key>VolumeUUID</key><string>` + id + `</string><key>VolumeUUID</key><string>` + id + `</string></dict></plist>`, `<plist><dict><key>VolumeUUID</key><string>invalid</string></dict></plist>`} {
		_, ok := parseVolumeUUID([]byte(raw))
		require.False(t, ok)
	}
}
