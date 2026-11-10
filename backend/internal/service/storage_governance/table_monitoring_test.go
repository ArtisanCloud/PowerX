package storagegov

import (
	"context"
	"encoding/json"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
	"time"
)

func monitorSnapshot(at time.Time, n int64) sampleDocument {
	return sampleDocument{ObservedAt: at, Database: &sampleDatabase{TableFingerprint: "cluster-schema-v1", Tables: []DatabaseTableSample{{ID: "capability_registry_invocation_traces", State: "observed", Bytes: &n}}}}
}
func TestTableGrowthRequiresContinuousSameDatabaseMeasurements(t *testing.T) {
	now := time.Now().UTC()
	s, _, _ := storageStateFixture(t)
	p, _ := s.tableMonitoringPolicy(nil)
	p.Enabled = true
	docs := []sampleDocument{monitorSnapshot(now.Add(-15*time.Minute), 1<<30), monitorSnapshot(now.Add(-10*time.Minute), 1100<<20), monitorSnapshot(now.Add(-5*time.Minute), 1200<<20), monitorSnapshot(now, 1400<<20)}
	rows, alerts, _ := EvaluateTableSnapshots(now, p, docs)
	require.Equal(t, int64(376<<20), *rows[0].NetChangeBytes)
	require.Equal(t, "table_growth_abnormal", alerts[0].Code)
	for _, mutate := range []func([]sampleDocument){
		func(d []sampleDocument) { d[1].Database.TableFingerprint = "other-cluster" },
		func(d []sampleDocument) { d[1].Database.Tables[0].State = "unavailable" },
		func(d []sampleDocument) {
			d[1].ObservedAt = now.Add(-14 * time.Minute)
			d[2].ObservedAt = now.Add(-2 * time.Minute)
		},
	} {
		copy := make([]sampleDocument, 4)
		for i := range docs {
			copy[i] = monitorSnapshot(docs[i].ObservedAt, *docs[i].Database.Tables[0].Bytes)
		}
		mutate(copy)
		rows, _, _ = EvaluateTableSnapshots(now, p, copy)
		require.Nil(t, rows[0].NetChangeBytes)
	}
	docs[3] = monitorSnapshot(now, 1<<29)
	rows, alerts, _ = EvaluateTableSnapshots(now, p, docs)
	require.Negative(t, *rows[0].NetChangeBytes)
	require.Empty(t, alerts)
	p.Enabled = false
	docs[3] = monitorSnapshot(now, 8<<30)
	rows, alerts, _ = EvaluateTableSnapshots(now, p, docs)
	require.Empty(t, alerts)
	require.Equal(t, "normal", rows[0].Level)
}

type monitoringCollector struct {
	tableFixtureCollector
	metricsCalls int
}

func (f *monitoringCollector) CollectDatabase(context.Context) DatabaseUsage {
	n := int64(4096)
	return DatabaseUsage{ClusterID: "owned-test-cluster", DatabaseBytes: &n}
}
func (f *monitoringCollector) CollectSystemTableMetrics(context.Context) ([]repo.SystemTableObservation, error) {
	f.metricsCalls++
	n := int64(4096)
	out := []repo.SystemTableObservation{}
	for _, d := range m.SystemTableCatalog() {
		out = append(out, repo.SystemTableObservation{SystemTableDefinition: d, Schema: "owned", State: "observed", TotalBytes: &n})
	}
	return out, nil
}

func TestTableMonitorPolicySamplingAndWorkerHealth(t *testing.T) {
	s, store, ctx := storageStateFixture(t)
	now := time.Now().UTC()
	s.opts.Now = func() time.Time { return now }
	c, e := budget.New(budget.Config{Directory: filepath.Join(t.TempDir(), "state"), FilesystemPath: t.TempDir(), HostID: "host", FilesystemID: "disk", InstanceID: "installed", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, e)
	s.opts.Budget = c
	collector := &monitoringCollector{}
	s.opts.DatabaseCollector = collector
	p, e := s.tableMonitoringPolicy(nil)
	require.NoError(t, e)
	require.False(t, p.Enabled)
	_, e = s.SaveTableMonitoring(context.Background(), 0, p)
	require.ErrorIs(t, e, ErrRootRequired)
	p.Enabled = true
	_, e = s.SaveTableMonitoring(ctx, 0, p)
	require.Error(t, e, "old JSON writer must remain blocked")
	s.opts.SamplingConfigured = true
	s.opts.SamplingWorkerEnabled = true
	p.Revision = 1
	_, e = c.Document(ctx, "table-monitoring", func([]byte) ([]byte, error) { return json.Marshal(p) })
	require.NoError(t, e) // legacy fixture for sampling evaluation; real writes use the control integration test.
	require.NoError(t, s.CaptureSample(ctx))
	require.Equal(t, 1, collector.metricsCalls)
	require.Zero(t, collector.calls)
	require.Len(t, store.samples, 1)
	var snap sampleDocument
	require.NoError(t, json.Unmarshal(store.samples[0].Document, &snap))
	require.Len(t, snap.Database.Tables, len(m.SystemTableCatalog()))
	out, e := s.DatabaseTableMonitoring(ctx)
	require.NoError(t, e)
	require.Equal(t, "observed", out.State)
	require.Len(t, out.Tables, len(m.SystemTableCatalog()))
	history, e := s.DatabaseTableHistory(ctx, m.SystemTableCatalog()[0].Name)
	require.NoError(t, e)
	require.Len(t, history.Points, 1)
	_, e = s.DatabaseTableHistory(ctx, "iam_users")
	require.ErrorIs(t, e, ErrSystemTableNotFound)
	now = now.Add(time.Hour)
	out, e = s.DatabaseTableMonitoring(ctx)
	require.NoError(t, e)
	require.Equal(t, "stale", out.State)
	require.Contains(t, alertCodes(out.Alerts), "table_sampling_stalled")
	s.opts.SamplingWorkerEnabled = false
	out, _ = s.DatabaseTableMonitoring(ctx)
	require.Equal(t, "unavailable", out.State)
	s.opts.LifecycleWorkerEnabled = true
	for i := 0; i < 3; i++ {
		id, e := s.startCleanupHealth(ctx)
		require.NoError(t, e)
		require.NoError(t, s.finishCleanupHealth(ctx, id, true))
	}
	h, e := s.cleanupHealth(ctx)
	require.NoError(t, e)
	require.Equal(t, 3, h.ConsecutiveFailures)
	require.Contains(t, alertCodes(evaluateCleanupHealth(now, p, h, true)), "cleanup_cycle_failed")
	id, e := s.startCleanupHealth(ctx)
	require.NoError(t, e)
	now = now.Add(6 * time.Minute)
	h, e = s.cleanupHealth(ctx)
	require.NoError(t, e)
	require.Contains(t, alertCodes(evaluateCleanupHealth(now, p, h, true)), "cleanup_cycle_unconfirmed")
	require.NoError(t, s.finishCleanupHealth(ctx, id, false))
	h, e = s.cleanupHealth(ctx)
	require.NoError(t, e)
	require.Zero(t, h.ConsecutiveFailures)
	require.NotNil(t, h.LastSuccessAt)
}
func alertCodes(a []CapacityAlert) []string {
	out := []string{}
	for _, v := range a {
		out = append(out, v.Code)
	}
	return out
}
func TestBacklogGraceAndUnknownAreNotHealthy(t *testing.T) {
	s, _, _ := storageStateFixture(t)
	p, _ := s.tableMonitoringPolicy(nil)
	now := time.Now()
	since := now.Add(-time.Hour)
	b := DatabaseBacklog{State: "observed", ObservedAt: &now, Since: &since, EligiblePresent: true}
	require.Contains(t, alertCodes(evaluateDatabaseBacklog(now, p, b)), "database_cleanup_backlog")
	b.Since = &now
	require.Empty(t, evaluateDatabaseBacklog(now, p, b))
	b.State = "unavailable"
	require.Contains(t, alertCodes(evaluateDatabaseBacklog(now, p, b)), "database_backlog_unavailable")
	b.State = "observed"
	b.ObservedAt = &since
	require.Contains(t, alertCodes(evaluateDatabaseBacklog(now, p, b)), "database_backlog_stale")
}
