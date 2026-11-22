package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	capm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestDatabaseRetentionNativeContinuesEmptyWindowsAndKeepsSingleCheckpoint(t *testing.T) {
	s, apply, db := retentionNative(t)
	p := adoptRetentionNative(t, s, apply)
	p = enableRetentionNative(t, s, apply, p.Revision)
	tenant := seedRetentionNative(t, db)
	// 受保护旧记录超过一批；末尾的真正过期记录仍须在同轮抵达。
	rows := make([]capm.InvocationTrace, caprepo.RetentionScanKeys+2)
	for i := range rows {
		rows[i] = capm.InvocationTrace{TenantUUID: tenant, TraceID: uuid.NewString(), Status: "completed", RecordClass: "legacy_unclassified"}
	}
	require.NoError(t, db.CreateInBatches(&rows, 100).Error)
	old := time.Now().UTC().AddDate(0, 0, -120)
	tail := capm.InvocationTrace{TenantUUID: tenant, TraceID: "eligible-tail", Status: "completed", RecordClass: capm.InvocationAuditClass, CompletedAt: &old}
	require.NoError(t, db.Create(&tail).Error)
	first, e := s.databaseBatch(context.Background(), 0, uuid.NewString(), true, true)
	require.NoError(t, e)
	require.False(t, first.Scan.Complete)
	require.EqualValues(t, 1, first.Counts.Traces)
	checkpoint, e := s.readRetentionCheckpoint(context.Background(), db)
	require.NoError(t, e)
	require.Equal(t, first.Scan.Next, checkpoint.Progress.Next)
	var failures []error
	s.runDatabaseRetentionCycle(context.Background(), func(err error) { failures = append(failures, err) })
	require.Empty(t, failures)
	checkpoint, e = s.readRetentionCheckpoint(context.Background(), db)
	require.NoError(t, e)
	require.True(t, checkpoint.Progress.Complete)
	var n int64
	require.NoError(t, db.Model(&capm.InvocationTrace{}).Where("uuid = ?", tail.UUID).Count(&n).Error)
	require.Zero(t, n)
	status, e := s.DatabaseRetention(controlRoot())
	require.NoError(t, e)
	require.Len(t, status.Runs, 2, "检查点不得成为一批一条的空执行回执")
	// 第二轮只有受保护记录：继续通过空窗口，复用检查点且不制造空回执。
	s.runDatabaseRetentionCycle(context.Background(), func(err error) { failures = append(failures, err) })
	require.Empty(t, failures)
	checkpoint, e = s.readRetentionCheckpoint(context.Background(), db)
	require.NoError(t, e)
	require.True(t, checkpoint.Progress.Complete)
	status, e = s.DatabaseRetention(controlRoot())
	require.NoError(t, e)
	require.Len(t, status.Runs, 2)
	row, e := s.scanLedger().Get(context.Background(), db, retentionCheckpointID)
	require.NoError(t, e)
	require.Equal(t, "checkpoint", row.State)
}

func TestDatabaseRetentionNativeCheckpointAndDeleteRollbackTogether(t *testing.T) {
	s, apply, db := retentionNative(t)
	p := adoptRetentionNative(t, s, apply)
	enableRetentionNative(t, s, apply, p.Revision)
	seedRetentionNative(t, db)
	key := uuid.NewString()
	name := "owned_checkpoint_rollback"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if row, ok := tx.Statement.Dest.(*m.CleanupRun); ok && row.RunID == key {
			tx.AddError(errors.New("owned receipt persistence failure"))
		}
	}))
	_, e := s.databaseBatch(context.Background(), 0, key, true, true)
	require.Error(t, e)
	require.EqualValues(t, 4, retentionRows(t, db))
	cp, e := s.readRetentionCheckpoint(context.Background(), db)
	require.NoError(t, e)
	require.Nil(t, cp)
	require.NoError(t, db.Callback().Create().Remove(name))
	run, e := s.databaseBatch(context.Background(), 0, key, true, true)
	require.NoError(t, e)
	require.EqualValues(t, 1, run.Counts.Traces)
	// 重复请求只读已提交回执，不重开一轮。
	again, e := s.databaseBatch(context.Background(), 0, key, true, true)
	require.NoError(t, e)
	require.Equal(t, run.Counts, again.Counts)
	before, e := s.readRetentionCheckpoint(context.Background(), db)
	require.NoError(t, e)
	_, e = s.databaseBatch(context.Background(), 0, key, true, true)
	require.NoError(t, e)
	after, e := s.readRetentionCheckpoint(context.Background(), db)
	require.NoError(t, e)
	require.Equal(t, before, after)
	// 损坏的检查点必须明确阻断，不能悄悄回到零或跳到末尾。
	row, e := s.scanLedger().Get(context.Background(), db, retentionCheckpointID)
	require.NoError(t, e)
	row.Document = datatypes.JSON(`{"revision":2}`)
	require.NoError(t, s.scanLedger().ReplaceCheckpoint(context.Background(), db, row))
	_, e = s.databaseBatch(context.Background(), 0, uuid.NewString(), true, true)
	require.Equal(t, "DATABASE_RETENTION_CHECKPOINT_INVALID", dto.CodeOf(e))
}

func TestDatabaseRetentionNativeConfirmationUsesFrozenWindowAndRejectsLegacyPreview(t *testing.T) {
	s, apply, db := retentionNative(t)
	p := adoptRetentionNative(t, s, apply)
	p = enableRetentionNative(t, s, apply, p.Revision)
	tenant := seedRetentionNative(t, db)
	preview, e := s.PreviewDatabaseRetention(controlRoot(), p.Revision)
	require.NoError(t, e)
	old := time.Now().UTC().AddDate(0, 0, -120)
	late := capm.InvocationTrace{TenantUUID: tenant, TraceID: uuid.NewString(), Status: "completed", RecordClass: capm.InvocationAuditClass, CompletedAt: &old}
	require.NoError(t, db.Create(&late).Error)
	run, e := s.RunConfirmedDatabaseRetention(controlRoot(), p.Revision, uuid.NewString(), preview.ID)
	require.NoError(t, e)
	require.Equal(t, preview.Scan.Window, run.Scan.Window)
	require.EqualValues(t, 1, run.Counts.Traces)
	var n int64
	require.NoError(t, db.Model(&capm.InvocationTrace{}).Where("uuid = ?", late.UUID).Count(&n).Error)
	require.EqualValues(t, 1, n)
	// 人工预览／确认不覆盖后台已推进的检查点。
	cp, e := s.readRetentionCheckpoint(context.Background(), db)
	require.NoError(t, e)
	require.Nil(t, cp)
	preview, e = s.PreviewDatabaseRetention(controlRoot(), p.Revision)
	require.NoError(t, e)
	_, e = s.databaseBatch(context.Background(), 0, uuid.NewString(), true, true)
	require.NoError(t, e)
	before, e := s.readRetentionCheckpoint(context.Background(), db)
	require.NoError(t, e)
	_, e = s.RunConfirmedDatabaseRetention(controlRoot(), p.Revision, uuid.NewString(), preview.ID)
	require.NoError(t, e)
	after, e := s.readRetentionCheckpoint(context.Background(), db)
	require.NoError(t, e)
	require.Equal(t, before, after, "确认旧预览不得覆盖后台的较新游标")
	preview, e = s.PreviewDatabaseRetention(controlRoot(), p.Revision)
	require.NoError(t, e)
	row, e := s.retentionLedger().Get(controlRoot(), db, preview.ID)
	require.NoError(t, e)
	require.NoError(t, db.Model(row).Update("document", gorm.Expr("document - 'scan'")).Error)
	_, e = s.RunConfirmedDatabaseRetention(controlRoot(), p.Revision, uuid.NewString(), preview.ID)
	require.Equal(t, "DATABASE_RETENTION_PREVIEW_EXPIRED", dto.CodeOf(e))
}

func TestDatabaseRetentionQueryTimeoutHasDistinctSafeCode(t *testing.T) {
	err := retentionExecutionError(&pgconn.PgError{Code: "57014", Message: "private SQL and connection details"})
	require.Equal(t, "DATABASE_RETENTION_QUERY_TIMEOUT", dto.CodeOf(err))
	require.NotContains(t, err.Error(), "private SQL")
}

func TestDatabaseRetentionNativeDefersBatchWithoutFullTimeBudget(t *testing.T) {
	s, apply, db := retentionNative(t)
	p := adoptRetentionNative(t, s, apply)
	enableRetentionNative(t, s, apply, p.Revision)
	seedRetentionNative(t, db)
	before := retentionRows(t, db)
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()
	var failures []error
	s.runDatabaseRetentionCycle(ctx, func(err error) { failures = append(failures, err) })
	require.Empty(t, failures)
	require.Equal(t, before, retentionRows(t, db), "剩余时间不足完整批次预算时，不得再启动删除事务")
	checkpoint, err := s.readRetentionCheckpoint(context.Background(), db)
	require.NoError(t, err)
	require.Nil(t, checkpoint)
	// 下一调度窗口仍按同一策略正常处理，不需要重新发布或回退进度。
	s.runDatabaseRetentionCycle(context.Background(), func(err error) { failures = append(failures, err) })
	require.Empty(t, failures)
	require.Equal(t, before-1, retentionRows(t, db))
}

func TestDatabaseRetentionNativeCycleStillReportsBatchFailure(t *testing.T) {
	s, apply, db := retentionNative(t)
	p := adoptRetentionNative(t, s, apply)
	enableRetentionNative(t, s, apply, p.Revision)
	seedRetentionNative(t, db)
	before := retentionRows(t, db)
	name := "owned_cycle_receipt_failure"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if row, ok := tx.Statement.Dest.(*m.CleanupRun); ok && row.State == "completed" {
			tx.AddError(context.DeadlineExceeded)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Create().Remove(name)) })
	var failures []error
	s.runDatabaseRetentionCycle(context.Background(), func(err error) { failures = append(failures, err) })
	require.Len(t, failures, 1, "预算充足时已启动批次的超时不能被当成正常让出")
	require.Equal(t, before, retentionRows(t, db), "失败批次的删除必须回滚")
	checkpoint, err := s.readRetentionCheckpoint(context.Background(), db)
	require.NoError(t, err)
	require.Nil(t, checkpoint, "失败批次不得提交游标")
}

func TestDatabaseRetentionExpiryRechecksSkippedPairsAndSharesCheckpoint(t *testing.T) {
	s, apply, db := retentionNative(t)
	p := adoptRetentionNative(t, s, apply)
	p = enableRetentionNative(t, s, apply, p.Revision)
	tenant := seedRetentionNative(t, db)
	now := time.Now().UTC()
	then := now.AddDate(0, 0, -4)
	finished := now.AddDate(0, 0, -p.SuccessDays-5)
	isolated := now.AddDate(0, 0, -p.QuarantineDays-1)
	notices := make([]capm.CapabilityEventPublication, p.BatchLimit+6)
	for i := range notices {
		notices[i] = capm.CapabilityEventPublication{TenantUUID: tenant, TraceID: uuid.NewString(), RecordClass: capm.InvocationNotificationClass, Status: "quarantined", TerminalAt: &isolated, Payload: datatypes.JSON(`{}`)}
	}
	require.NoError(t, db.CreateInBatches(&notices, 100).Error)
	traces := make([]capm.InvocationTrace, len(notices))
	for i, notice := range notices {
		traces[i] = capm.InvocationTrace{TenantUUID: tenant, TraceID: notice.TraceID, RecordClass: capm.InvocationAuditClass, Status: "completed", CompletedAt: &finished, EventPublicationID: &notices[i].UUID}
	}
	require.NoError(t, db.CreateInBatches(&traces, 100).Error)
	// 模拟旧版已扫完审计表、通知游标还在前进的持久检查点（不含新增字段）。
	ctx := context.Background()
	r := caprepo.NewRetentionRepository(db)
	high, err := r.ScanHighwater(ctx)
	require.NoError(t, err)
	scan := caprepo.RetentionProgress{Window: caprepo.RetentionWindow{End: high, Highwater: high}, Next: caprepo.RetentionPosition{Traces: high.Traces}}
	cutoffs := s.retentionCutoffs(p, then)
	raw, err := json.Marshal(map[string]any{"revision": p.Revision, "cutoffs": cutoffs, "progress": scan})
	require.NoError(t, err)
	require.NoError(t, s.scanLedger().Create(ctx, db, &m.CleanupRun{RunID: retentionCheckpointID, Version: 1, State: "checkpoint", ExpiresAt: now.AddDate(0, 0, 30), Document: raw}))
	normal, err := s.PreviewDatabaseRetention(controlRoot(), p.Revision)
	require.NoError(t, err)
	require.True(t, normal.Cutoffs.Quarantine.After(isolated))
	require.EqualValues(t, 0, normal.Counts.Traces, "普通窗口已经走过该审计记录")
	preview, err := s.PreviewDatabaseRetentionExpiry(controlRoot(), p.Revision)
	require.NoError(t, err)
	require.True(t, preview.Cutoffs.ExpiryOnly)
	require.EqualValues(t, p.BatchLimit, preview.Counts.Traces)
	require.EqualValues(t, p.BatchLimit-1, preview.Counts.Events)
	require.EqualValues(t, 0, preview.Counts.Quarantined)
	// 两个调用方都拿到相同窗口。落后的确认不能把检查点写回旧位置。
	stale, err := s.PreviewDatabaseRetentionExpiry(controlRoot(), p.Revision)
	require.NoError(t, err)
	id := uuid.NewString()
	run, err := s.RunConfirmedDatabaseRetention(controlRoot(), p.Revision, id, preview.ID)
	require.NoError(t, err)
	require.EqualValues(t, p.BatchLimit, run.Counts.Traces)
	require.EqualValues(t, p.BatchLimit-1, run.Counts.Events)
	again, err := s.RunConfirmedDatabaseRetention(controlRoot(), p.Revision, id, preview.ID)
	require.NoError(t, err)
	require.Equal(t, run.ID, again.ID)
	_, err = s.RunConfirmedDatabaseRetention(controlRoot(), p.Revision, uuid.NewString(), stale.ID)
	require.Equal(t, "DATABASE_RETENTION_PREVIEW_EXPIRED", dto.CodeOf(err))
	checkpoint, err := s.readRetentionCheckpoint(ctx, db)
	require.NoError(t, err)
	require.Equal(t, scan, checkpoint.Progress)
	next, err := s.databaseBatchConfirmed(ctx, 0, uuid.NewString(), "", true, true, true)
	require.NoError(t, err)
	require.Equal(t, id, next.PreviousExpiryRun)
	require.EqualValues(t, 7, next.Counts.Traces)
	require.EqualValues(t, 7, next.Counts.Events)
	require.Zero(t, next.Counts.Quarantined)
	var n int64
	require.NoError(t, db.Unscoped().Model(&capm.CapabilityEventPublication{}).Where("topic = ?", "business.order.created").Count(&n).Error)
	require.EqualValues(t, 1, n)
}
