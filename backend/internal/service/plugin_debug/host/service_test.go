package host

import (
	"context"
	"testing"
	"time"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"github.com/stretchr/testify/require"
)

type fakeAudit struct {
	count int
}

func (f *fakeAudit) Emit(ctx context.Context, evt *dbm.AuditEvent) error {
	f.count++
	return nil
}

func (f *fakeAudit) Close() {}

func TestRegisterAndPruneHosts(t *testing.T) {
	a := &fakeAudit{}
	now := time.Now()
	clock := func() time.Time { return now }
	svc := NewService(a, Options{
		Component:     "test",
		ConfigPath:    "",
		PruneInterval: time.Minute,
		Now:           clock,
	})
	defer svc.Close()

	session := svc.RegisterMockHost(context.Background(), "plugin.demo", "local", 0, 0, 0, []string{"debug.hot_reload"})
	require.Equal(t, "plugin.demo", session.PluginID)
	require.Equal(t, defaultHTTPPort, session.HTTPPort)
	require.Equal(t, defaultGRPCPort, session.GRPCPort)
	require.Equal(t, time.Duration(defaultTTLSeconds)*time.Second, session.TTL)

	// Force expiry and pruning.
	now = now.Add(time.Duration(defaultTTLSeconds+1) * time.Second)
	svc.pruneExpired()
	require.Empty(t, svc.ListActive())
}

func TestRecordReloadWithVersionMismatch(t *testing.T) {
	a := &fakeAudit{}
	now := time.Now()
	clock := func() time.Time { return now }
	svc := NewService(a, Options{
		Component:     "test",
		PruneInterval: time.Minute,
		Now:           clock,
	})
	defer svc.Close()

	session := svc.RegisterMockHost(context.Background(), "plugin.demo", "local", 0, 0, 0, nil)
	svc.RecordReload(context.Background(), ReloadEvent{
		SessionID:       session.ID,
		Duration:        1500 * time.Millisecond,
		Success:         true,
		Sequence:        1,
		VersionMismatch: true,
	})

	// No panic and audit counter increments twice (start + reload).
	require.Equal(t, 2, a.count)
}
