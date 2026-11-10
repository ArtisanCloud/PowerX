package storagegov

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func monitoringNative(t *testing.T) (*Service, *ControlService, *taskqueue.StreamDriver, *gorm.DB) {
	t.Helper()
	db, queue := ownedControlPostgres(t)
	dir := t.TempDir()
	c, e := budget.New(budget.Config{Directory: filepath.Join(dir, "runtime"), FilesystemPath: dir, HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-install", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, e)
	var s *Service
	control, e := NewControlService(db, ControlOptions{Scope: ControlScope{"owned-host", "owned-fs", "owned-install"}, Queue: queue, Changed: func() {
		if s != nil {
			s.PublishChange("configuration_changed")
		}
	}})
	require.NoError(t, e)
	s, e = NewService(Options{Identity: Identity{HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-install", Environment: "test"}, FilesystemPath: dir, Collector: &fixtureCollector{}, DB: db, Budget: c, Control: control, ControlWorkerEnabled: true, SamplingConfigured: true, SamplingWorkerEnabled: true, DatabaseCollector: &monitoringCollector{}})
	require.NoError(t, e)
	p, _ := s.tableMonitoringPolicy(nil)
	p.Revision = 7
	p.Enabled = true
	p.Actor = 11
	p.UpdatedAt = time.Now().UTC()
	_, e = c.Document(context.Background(), "table-monitoring", func([]byte) ([]byte, error) { return json.Marshal(p) })
	require.NoError(t, e)
	return s, control, queue, db
}
func applyMonitoring(t *testing.T, c *ControlService, q *taskqueue.StreamDriver) {
	t.Helper()
	items, e := q.Dequeue(context.Background(), controlQueueTenant, c.subscriber(), "owned-monitoring", "owned-worker", 10, time.Millisecond)
	require.NoError(t, e)
	require.NotEmpty(t, items)
	for _, item := range items {
		require.NoError(t, c.ApplyDelivery(context.Background(), item))
	}
}
func TestTableMonitoringNativePostgresAdoptionSaveReceiptAndProjection(t *testing.T) {
	s, c, q, db := monitoringNative(t)
	ctx := controlRoot()
	legacy, e := s.opts.Budget.Document(ctx, "table-monitoring", nil)
	require.NoError(t, e)
	out, e := s.DatabaseTableMonitoring(ctx)
	require.NoError(t, e)
	require.Equal(t, "legacy", out.Configuration.Authority)
	require.True(t, out.Policy.Enabled)
	_, e = s.SaveTableMonitoring(ctx, 7, out.Policy)
	require.Equal(t, "STORAGE_CONTROL_PRECONDITION_REQUIRED", dto.CodeOf(e))
	_, e = s.AdoptTableMonitoring(ctx, 6, out.Configuration.LegacyDigest, uuid.NewString())
	require.Equal(t, "STORAGE_TABLE_MONITORING_LEGACY_CHANGED", dto.CodeOf(e))
	key := uuid.NewString()
	saved, e := s.AdoptTableMonitoring(ctx, 7, out.Configuration.LegacyDigest, key)
	require.NoError(t, e)
	require.False(t, saved.OperationPolicy.Enabled)
	require.Equal(t, "pending", saved.Configuration.State)
	require.Zero(t, saved.Configuration.AppliedRevision)
	applyMonitoring(t, c, q)
	out, e = s.DatabaseTableMonitoring(ctx)
	require.NoError(t, e)
	require.False(t, out.Policy.Enabled)
	require.Equal(t, "applied", out.Configuration.State)
	require.EqualValues(t, 1, *out.Configuration.SnapshotRevision)
	p := *out.Configuration.DesiredPolicy
	p.Enabled = true
	p.GrowthBytes = 128 << 20
	key = uuid.NewString()
	saved, e = s.SaveTableMonitoringConfiguration(ctx, 1, key, p)
	require.NoError(t, e)
	require.EqualValues(t, 2, saved.OperationRevision)
	out, e = s.DatabaseTableMonitoring(ctx)
	require.NoError(t, e)
	require.False(t, out.Policy.Enabled, "pending enable cannot use the old snapshot")
	receipt, e := s.TableMonitoringRequest(ctx, key)
	require.NoError(t, e)
	require.Equal(t, p, receipt.OperationPolicy)
	replay, e := s.SaveTableMonitoringConfiguration(ctx, 1, key, p)
	require.NoError(t, e)
	require.EqualValues(t, 2, replay.OperationRevision)
	var count int64
	require.NoError(t, db.Model(&m.ControlRevision{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
	applyMonitoring(t, c, q)
	out, e = s.DatabaseTableMonitoring(ctx)
	require.NoError(t, e)
	require.True(t, out.Policy.Enabled)
	require.EqualValues(t, 2, out.Policy.Revision)
	p.Enabled = false
	_, e = s.SaveTableMonitoringConfiguration(ctx, 2, uuid.NewString(), p)
	require.NoError(t, e)
	out, e = s.DatabaseTableMonitoring(ctx)
	require.NoError(t, e)
	require.False(t, out.Policy.Enabled, "a pending disable fences the last enabled snapshot")
	require.EqualValues(t, 2, out.Configuration.AppliedRevision)
	applyMonitoring(t, c, q)
	after, e := s.opts.Budget.Document(ctx, "table-monitoring", nil)
	require.NoError(t, e)
	require.Equal(t, legacy, after, "legacy policy remains immutable evidence")
	_, e = s.SaveTableMonitoringConfiguration(ctx, 1, uuid.NewString(), p)
	require.Equal(t, "STORAGE_CONTROL_REVISION_CONFLICT", dto.CodeOf(e))
}
func TestTableMonitoringNativePostgresSnapshotTamperAndMissingAuthorityFailClosed(t *testing.T) {
	s, c, q, db := monitoringNative(t)
	ctx := controlRoot()
	out, _ := s.DatabaseTableMonitoring(ctx)
	_, e := s.AdoptTableMonitoring(ctx, 7, out.Configuration.LegacyDigest, uuid.NewString())
	require.NoError(t, e)
	applyMonitoring(t, c, q)
	out, _ = s.DatabaseTableMonitoring(ctx)
	p := *out.Configuration.DesiredPolicy
	p.Enabled = true
	_, e = s.SaveTableMonitoringConfiguration(ctx, 1, uuid.NewString(), p)
	require.NoError(t, e)
	applyMonitoring(t, c, q)
	_, e = s.opts.Budget.Document(ctx, "table-monitoring-control", func(raw []byte) ([]byte, error) {
		var projection monitoringProjection
		require.NoError(t, json.Unmarshal(raw, &projection))
		projection.Scope.InstanceID = "foreign-instance"
		return json.Marshal(projection)
	})
	require.NoError(t, e)
	out, _ = s.DatabaseTableMonitoring(ctx)
	require.Equal(t, "unavailable", out.State)
	require.False(t, out.Policy.Enabled)
	require.Equal(t, "unconfirmed", out.Configuration.State)
	require.NoError(t, db.Migrator().DropTable(&m.ControlConfiguration{}))
	out, _ = s.DatabaseTableMonitoring(ctx)
	require.Equal(t, "unavailable", out.State)
	require.False(t, out.Policy.Enabled, "missing DB must never revive the enabled legacy JSON")
}

func TestTableMonitoringNativePostgresAdoptionReceiptSurvivesLaterLegacyDrift(t *testing.T) {
	s, c, q, _ := monitoringNative(t)
	ctx := controlRoot()
	status, _ := s.DatabaseTableMonitoring(ctx)
	origin := *status.Configuration
	key := uuid.NewString()
	_, e := s.AdoptTableMonitoring(ctx, origin.LegacyRevision, origin.LegacyDigest, key)
	require.NoError(t, e)
	applyMonitoring(t, c, q)
	_, e = s.opts.Budget.Document(ctx, "table-monitoring", func(raw []byte) ([]byte, error) {
		var old TableMonitoringPolicy
		require.NoError(t, json.Unmarshal(raw, &old))
		old.Revision++
		return json.Marshal(old)
	})
	require.NoError(t, e)
	result, e := s.AdoptTableMonitoring(ctx, origin.LegacyRevision, origin.LegacyDigest, key)
	require.NoError(t, e)
	require.EqualValues(t, 1, result.OperationRevision)
	require.False(t, result.OperationPolicy.Enabled)
	_, e = s.AdoptTableMonitoring(ctx, origin.LegacyRevision, origin.LegacyDigest, uuid.NewString())
	require.Equal(t, "STORAGE_TABLE_MONITORING_LEGACY_CHANGED", dto.CodeOf(e))
}

func TestTableMonitoringNativePostgresExitedWorkerRequiresRestart(t *testing.T) {
	s, c, _, _ := monitoringNative(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.StartControl(ctx, func(error) {})
	select {
	case <-c.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("owned worker did not exit")
	}
	status, _ := s.DatabaseTableMonitoring(controlRoot())
	require.False(t, status.Configuration.Ready)
	_, e := s.AdoptTableMonitoring(controlRoot(), status.Configuration.LegacyRevision, status.Configuration.LegacyDigest, uuid.NewString())
	require.Equal(t, "STORAGE_CONTROL_WORKER_UNAVAILABLE", dto.CodeOf(e))
}
