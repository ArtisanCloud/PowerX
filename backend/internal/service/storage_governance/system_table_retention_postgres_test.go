package storagegov

import (
	"context"
	"encoding/json"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// 使用同一正式 runtime 周期及自有 PostgreSQL；推进受控时钟，不代替长期运行证据。
func TestSystemTableRetentionRuntimeNativePostgres(t *testing.T) {
	s, apply, db := systemRetentionNative(t)
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -45)
	rows := []m.HealthProbeResult{}
	for i := 0; i < 7; i++ {
		rows = append(rows, m.HealthProbeResult{AdapterID: "primary", ProbeWindowStart: old.Add(time.Duration(i) * time.Hour), Status: "healthy", NextRetryAt: old, CreatedAt: old, UpdatedAt: old})
	}
	for _, extra := range []m.HealthProbeResult{{AdapterID: "primary", ProbeWindowStart: now, Status: "healthy", NextRetryAt: now, CreatedAt: now, UpdatedAt: now}, {AdapterID: "primary", ProbeWindowStart: old.Add(8 * time.Hour), Status: "pending", NextRetryAt: old, CreatedAt: old, UpdatedAt: old}, {AdapterID: "primary", ProbeWindowStart: old.Add(9 * time.Hour), Status: "unhealthy", NextRetryAt: now.Add(time.Hour), CreatedAt: old, UpdatedAt: old}} {
		rows = append(rows, extra)
	}
	require.NoError(t, db.Create(&rows).Error)
	s.opts.Now = func() time.Time { return now }
	ctx := controlRoot()
	adoptSystemNative(t, s, apply, repo.ProbeRetentionID)
	settings := SystemTableRetentionSettings{RetentionDays: 30, BatchLimit: 1}
	_, e := s.ChangeSystemTableRetention(ctx, repo.ProbeRetentionID, SystemTableRetentionChange{RequestID: uuid.NewString(), ExpectedRevision: 1, Action: "save", Settings: &settings, Confirm: true})
	require.NoError(t, e)
	apply()
	enableSystemNative(t, s, apply, repo.ProbeRetentionID, 2)
	failures := []error{}
	s.runRuntimeLifecycle(context.Background(), func(e error) { failures = append(failures, e) })
	require.Empty(t, failures)
	var count int64
	require.NoError(t, db.Unscoped().Model(&m.HealthProbeResult{}).Count(&count).Error)
	require.EqualValues(t, 5, count)
	now = now.Add(15 * time.Minute)
	s.runRuntimeLifecycle(context.Background(), func(e error) { failures = append(failures, e) })
	require.Empty(t, failures)
	require.NoError(t, db.Unscoped().Model(&m.HealthProbeResult{}).Count(&count).Error)
	require.EqualValues(t, 3, count)
	health, e := s.cleanupHealth(ctx)
	require.NoError(t, e)
	require.Equal(t, "completed", health.State)
	require.Zero(t, health.ConsecutiveFailures)
	status, e := s.SystemTableRetention(ctx, repo.ProbeRetentionID)
	require.NoError(t, e)
	require.NotEmpty(t, status.Runs)
	require.EqualValues(t, 7, status.Runs[0].Policy.Actor)
	require.Zero(t, status.Runs[0].Actor)
	// 持久化的未知意图阻止新的自动批次，不能被新周期或新请求覆盖。
	_, e = s.opts.Budget.Document(ctx, "system-table-retention", func(raw []byte) ([]byte, error) {
		d, e := s.systemTableDocument(raw)
		if e != nil {
			return nil, e
		}
		d.Runs = append([]SystemTableRetentionRun{{ID: uuid.NewString(), TableID: repo.ProbeRetentionID, Revision: d.Revision, State: "unconfirmed", At: now, Cutoff: old, Policy: d.Policies[repo.ProbeRetentionID]}}, d.Runs...)
		return json.Marshal(d)
	})
	require.NoError(t, e)
	now = now.Add(15 * time.Minute)
	s.runRuntimeLifecycle(context.Background(), func(e error) { failures = append(failures, e) })
	require.NotEmpty(t, failures)
	health, e = s.cleanupHealth(ctx)
	require.NoError(t, e)
	require.Equal(t, "failed", health.State)
	require.Equal(t, 1, health.ConsecutiveFailures)
	require.NoError(t, db.Unscoped().Model(&m.HealthProbeResult{}).Count(&count).Error)
	require.EqualValues(t, 3, count)
}
