package opsintegration

import (
	"context"
	"testing"

	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	deployops "github.com/ArtisanCloud/PowerX/internal/service/deploy_ops"
	migrationops "github.com/ArtisanCloud/PowerX/internal/service/migration_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	auditrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTraceabilityAcrossOpsDomains(t *testing.T) {
	db := setupTraceabilityDB(t)
	traceID := "trace-ops-e2e-001"

	ctx := context.Background()
	ctx = reqctx.WithTenantUUID(ctx, "tenant-traceability")
	ctx = reqctx.WithTraceID(ctx, traceID)

	deploySvc := deployops.NewService(db)
	pluginSvc := deployops.NewPluginLifecycleService(db)
	backupPolicySvc := backupops.NewPolicyService(db)
	backupJobSvc := backupops.NewJobService(db)
	restoreSvc := backupops.NewRestoreDrillService(db)
	migrationSvc := migrationops.NewService(db)

	release, err := deploySvc.TriggerRelease(ctx, deployops.ReleaseRequest{
		Environment:     "prod",
		BackendVersion:  "v2.0.1",
		WebAdminVersion: "v2.0.1",
		Mode:            deployops.DeployModeDocker,
		Operator:        "trace-test",
		TraceID:         traceID,
		ApprovalTickets: 2,
	})
	require.NoError(t, err)
	require.Equal(t, traceID, release.TraceID)

	pluginAudit, err := pluginSvc.TriggerAction(ctx, deployops.PluginLifecycleActionRequest{
		PluginID:    "plugin.trace",
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Action:      "switch",
		Reason:      "traceability",
		Operator:    "trace-test",
		TraceID:     traceID,
	})
	require.NoError(t, err)
	require.Equal(t, traceID, pluginAudit.TraceID)

	policy, err := backupPolicySvc.UpsertPolicy(ctx, backupops.UpsertPolicyRequest{
		Name:          "trace-policy",
		BackupType:    "logical",
		Schedule:      "0 2 * * *",
		RetentionDays: 7,
		Enabled:       true,
		StorageTarget: "s3://powerx-backup/trace",
		Operator:      "trace-test",
		TraceID:       traceID,
	})
	require.NoError(t, err)

	job, err := backupJobSvc.TriggerJob(ctx, backupops.TriggerJobRequest{
		PolicyID: policy.ID,
		Operator: "trace-test",
		TraceID:  traceID,
	})
	require.NoError(t, err)
	require.Equal(t, traceID, job.TraceID)

	drill, err := restoreSvc.Trigger(ctx, backupops.TriggerRestoreDrillRequest{
		SourceJobID: job.ID,
		Operator:    "trace-test",
		TraceID:     traceID,
	})
	require.NoError(t, err)
	require.Equal(t, traceID, drill.TraceID)

	migrationRecord, err := migrationSvc.TriggerMigration(ctx, migrationops.TriggerRequest{
		SourceEnv: "prod-a",
		TargetEnv: "prod-b",
		DryRun:    true,
		Operator:  "trace-test",
		TraceID:   traceID,
	})
	require.NoError(t, err)
	require.Equal(t, traceID, migrationRecord.TraceID)

	audits, total, err := auditrepo.NewAuditEventRepository(db).List(ctx, auditrepo.ListFilter{
		TenantUUID:    "tenant-traceability",
		CorrelationID: traceID,
		Page:          1,
		Size:          100,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, int64(5))
	require.NotEmpty(t, audits)
	for _, row := range audits {
		require.Equal(t, traceID, row.CorrelationID)
	}
}

func setupTraceabilityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := model.PowerXSchema
	model.PowerXSchema = "main"
	t.Cleanup(func() { model.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&auditmodel.AuditEvent{},
		&modelops.DeployReleaseRecord{},
		&modelops.ApprovalPolicyProfile{},
		&modelops.PluginLifecycleAudit{},
		&modelops.BackupPolicy{},
		&modelops.BackupJob{},
		&modelops.RestoreDrillRecord{},
		&modelops.MigrationRunbookRecord{},
	))
	return db
}
