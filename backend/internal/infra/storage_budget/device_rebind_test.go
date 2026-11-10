package storagebudget

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func saveChangedDevice(t *testing.T, c *Coordinator, edit func(*ledger)) []byte {
	t.Helper()
	p := filepath.Join(c.cfg.Directory, "budget.json")
	raw, e := os.ReadFile(p)
	require.NoError(t, e)
	var d ledger
	require.NoError(t, json.Unmarshal(raw, &d))
	d.Device++
	if edit != nil {
		edit(&d)
	}
	raw, e = json.Marshal(d)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(p, raw, 0600))
	return raw
}
func TestStartupRebindRequiresStableProofAndIdleDisabledState(t *testing.T) {
	c := fixture(t, "dev", "", nil)
	before := saveChangedDevice(t, c, nil)
	policy := []byte(`{"version":1,"policies":{"logs":{"enabled":false}},"runs":[{"state":"completed"}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(c.cfg.Directory, "lifecycle.json"), policy, 0600))
	restarted, e := newCoordinator(c.cfg, func(path, id string) bool { return path == c.cfg.FilesystemPath && id == "disk" })
	require.NoError(t, e)
	raw, e := os.ReadFile(filepath.Join(c.cfg.Directory, "budget.json"))
	require.NoError(t, e)
	var old, current ledger
	require.NoError(t, json.Unmarshal(before, &old))
	require.NoError(t, json.Unmarshal(raw, &current))
	require.Equal(t, c.device, current.Device)
	require.Equal(t, old.Policy, current.Policy)
	require.Empty(t, current.Reservations)
	require.Len(t, current.DeviceRebindings, 1)
	require.Equal(t, old.Device, current.DeviceRebindings[0].PreviousDevice)
	require.Equal(t, "dev", current.DeviceRebindings[0].InstanceID)
	retained, e := os.ReadFile(filepath.Join(c.cfg.Directory, "lifecycle.json"))
	require.NoError(t, e)
	require.Equal(t, policy, retained)
	_, e = restarted.Status(context.Background())
	require.NoError(t, e)
	// 运行中不能自动接受被修改的设备绑定。
	saveChangedDevice(t, restarted, nil)
	_, e = restarted.Status(context.Background())
	require.ErrorIs(t, e, ErrDeviceChanged)
}
func TestRebindPreservesLedgerOnUnknownVolumeReservationsOrCleanup(t *testing.T) {
	tests := []struct {
		name, document, body string
		proof                bool
		edit                 func(*ledger)
	}{
		{name: "unproven-volume"},
		{name: "active-reservation", proof: true, edit: func(d *ledger) {
			d.Reservations["owned"] = Reservation{ID: "owned", InstanceID: "dev", Operation: "fixture", ExpiresAt: time.Now().Add(time.Hour)}
		}},
		{name: "expired-reservation", proof: true, edit: func(d *ledger) {
			d.Reservations["expired"] = Reservation{ID: "expired", ExpiresAt: time.Now().Add(-time.Hour)}
		}},
		{name: "enabled-file", proof: true, document: "lifecycle.json", body: `{"version":1,"policies":{"logs":{"enabled":true}},"runs":[]}`},
		{name: "enabled-database", proof: true, document: "database-retention.json", body: `{"policy":{"enabled":true},"runs":[]}`},
		{name: "enabled-typed", proof: true, document: "system-table-retention.json", body: `{"policies":{"probes":{"enabled":true}},"runs":[]}`},
		{name: "unconfirmed", proof: true, document: "lifecycle.json", body: `{"version":1,"policies":{},"runs":[{"state":"unconfirmed"}]}`},
		{name: "pending-quarantine", proof: true, document: "lifecycle.json", body: `{"version":1,"policies":{},"runs":[{"state":"failed","pending_name":"fixture"}]}`},
		{name: "missing-enabled", proof: true, document: "lifecycle.json", body: `{"version":1,"policies":{"logs":{}},"runs":[]}`},
		{name: "corrupt-state", proof: true, document: "lifecycle.json", body: `{`},
		{name: "ownership", proof: true, document: "ownership.json", body: `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fixture(t, "dev", "", nil)
			before := saveChangedDevice(t, c, tt.edit)
			if tt.document != "" {
				require.NoError(t, os.WriteFile(filepath.Join(c.cfg.Directory, tt.document), []byte(tt.body), 0600))
			}
			_, e := newCoordinator(c.cfg, func(string, string) bool { return tt.proof })
			require.ErrorIs(t, e, ErrDeviceChanged)
			after, e := os.ReadFile(filepath.Join(c.cfg.Directory, "budget.json"))
			require.NoError(t, e)
			require.Equal(t, before, after)
		})
	}
}
