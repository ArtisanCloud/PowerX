package capability_registry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	tenants "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestRetentionScanCrossesProtectedWindowsAndDrainsLimitedCandidates(t *testing.T) {
	db := retentionDB(t)
	r := NewRetentionRepository(db)
	ctx := context.Background()
	tenant := &tenants.Tenant{Key: "scan", Name: "scan"}
	require.NoError(t, db.Create(tenant).Error)
	old := time.Now().UTC().AddDate(0, 0, -120)
	rows := make([]m.InvocationTrace, RetentionScanKeys+3)
	for i := range rows {
		rows[i] = m.InvocationTrace{TenantUUID: tenant.UUID.String(), TraceID: fmt.Sprint(i), Status: "completed", RecordClass: "legacy_unclassified", CompletedAt: &old}
		if i >= RetentionScanKeys {
			rows[i].RecordClass = m.InvocationAuditClass
		}
	}
	require.NoError(t, db.CreateInBatches(&rows, 100).Error)
	c := RetentionCutoffs{Success: old.AddDate(0, 0, 30), Failure: old.AddDate(0, 0, 1), Pending: old, Quarantine: old, Limit: 1}
	high, e := r.ScanHighwater(ctx)
	require.NoError(t, e)
	w, e := r.ScanWindow(ctx, RetentionPosition{}, high)
	require.NoError(t, e)
	require.EqualValues(t, RetentionScanKeys, w.End.Traces)
	counts, progress, e := r.BatchWindow(ctx, c, true, w)
	require.NoError(t, e)
	require.Zero(t, counts.Traces)
	require.False(t, progress.Complete, "空窗口不能冒充全库无候选")
	// 本轮上界以后才写入的记录不能被既有预览／轮次包含。
	late := m.InvocationTrace{TenantUUID: tenant.UUID.String(), TraceID: "after-highwater", Status: "completed", RecordClass: m.InvocationAuditClass, CompletedAt: &old}
	require.NoError(t, db.Create(&late).Error)
	for i := 0; i < 3; i++ {
		w, e = r.ScanWindow(ctx, progress.Next, high)
		require.NoError(t, e)
		preview, _, err := r.BatchWindow(ctx, c, false, w)
		require.NoError(t, err)
		require.EqualValues(t, 1, preview.Traces)
		counts, progress, e = r.BatchWindow(ctx, c, true, w)
		require.NoError(t, e)
		require.True(t, progress.Valid())
		require.EqualValues(t, 1, counts.Traces)
	}
	require.True(t, progress.Complete)
	var n int64
	require.NoError(t, db.Unscoped().Model(&m.InvocationTrace{}).Count(&n).Error)
	require.EqualValues(t, RetentionScanKeys+1, n)
	// 账本篡改扩大窗口也不得绕过每批检查上限。
	_, _, e = r.BatchWindow(ctx, c, true, RetentionWindow{End: RetentionPosition{Traces: int64(late.ID)}, Highwater: RetentionPosition{Traces: int64(late.ID)}})
	require.EqualError(t, e, "invalid_retention_window")
}

func TestRetentionScanSparseKeysAndQueryTimeoutClassification(t *testing.T) {
	db := retentionDB(t)
	row := m.InvocationTrace{TraceID: "sparse", Status: "completed", TenantUUID: "malformed-tenant"}
	row.ID = 1 << 40
	require.NoError(t, db.Create(&row).Error)
	r := NewRetentionRepository(db)
	high, e := r.ScanHighwater(context.Background())
	require.NoError(t, e)
	w, e := r.ScanWindow(context.Background(), RetentionPosition{}, high)
	require.NoError(t, e)
	require.Equal(t, high, w.End)
	require.True(t, IsRetentionQueryTimeout(fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "57014"})))
	require.True(t, IsRetentionQueryTimeout(context.DeadlineExceeded))
	require.False(t, IsRetentionQueryTimeout(errors.New("SQLSTATE 57014")))
	require.False(t, IsRetentionQueryTimeout(&pgconn.PgError{Code: "42P01"}))
}

func TestRetentionScanPostgresScale(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("POWERX_RETENTION_SCALE_ROWS"))
	if n < 100000 || n > 2000000 {
		t.Skip("explicit owned PostgreSQL scale fixture only")
	}
	db := retentionDB(t)
	require.Equal(t, "postgres", db.Dialector.Name())
	tenant := &tenants.Tenant{Key: "scale", Name: "scale"}
	require.NoError(t, db.Create(tenant).Error)
	// 未能核验负载的历史通知及其终态链路，大量受保护记录不得占住下一窗口。
	events, traces := m.CapabilityEventPublication{}.TableName(), m.InvocationTrace{}.TableName()
	require.NoError(t, db.Exec("INSERT INTO "+events+" (uuid,tenant_uuid,trace_id,topic,payload,status,created_at) SELECT md5('event-'||g)::uuid,?,g::text,'_topic.integration.gateway.invocation.succeeded','{}'::jsonb,'pending',now()-interval '120 days' FROM generate_series(1,?) g", tenant.UUID.String(), n).Error)
	require.NoError(t, db.Exec("INSERT INTO "+traces+" (uuid,tenant_uuid,trace_id,plugin_id,capability_id,protocol_used,status,event_publication_id,updated_at) SELECT md5('trace-'||g)::uuid,?,g::text,'scale','scale','http','completed',md5('event-'||g)::uuid,now()-interval '120 days' FROM generate_series(1,?) g", tenant.UUID.String(), n).Error)
	require.NoError(t, db.Exec("ANALYZE "+events).Error)
	require.NoError(t, db.Exec("ANALYZE "+traces).Error)
	r := NewRetentionRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	started := time.Now()
	high, e := r.ScanHighwater(ctx)
	require.NoError(t, e)
	w, e := r.ScanWindow(ctx, RetentionPosition{}, high)
	require.NoError(t, e)
	c := RetentionCutoffs{Success: time.Now().AddDate(0, 0, -30), Failure: time.Now().AddDate(0, 0, -90), Pending: time.Now().Add(-24 * time.Hour), Quarantine: time.Now().AddDate(0, 0, -30), Limit: 200, AdoptLegacy: true}
	counts, progress, e := r.BatchWindow(ctx, c, false, w)
	require.NoError(t, e)
	require.Equal(t, RetentionBatch{}, counts)
	require.False(t, progress.Complete)
	require.EqualValues(t, RetentionScanKeys, progress.Next.Traces)
	require.EqualValues(t, RetentionScanKeys, progress.Next.Events)
	t.Logf("rows_per_table=%d protected_window=%d preview_elapsed=%s", n, RetentionScanKeys, time.Since(started))
}
