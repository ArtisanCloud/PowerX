package opsintegration

import (
	"context"
	"testing"

	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBackupRestoreFlow(t *testing.T) {
	db := setupBackupDB(t)
	ctx := context.Background()

	policySvc := backupops.NewPolicyService(db)
	jobSvc := backupops.NewJobService(db)
	restoreSvc := backupops.NewRestoreDrillService(db)

	policy, err := policySvc.UpsertPolicy(ctx, backupops.UpsertPolicyRequest{
		Name:          "daily-main",
		BackupType:    "logical",
		Schedule:      "0 2 * * *",
		RetentionDays: 30,
		Enabled:       true,
		StorageTarget: "s3://powerx-backup/main",
		Operator:      "integration",
	})
	require.NoError(t, err)

	job, err := jobSvc.TriggerJob(ctx, backupops.TriggerJobRequest{PolicyID: policy.ID, Operator: "integration"})
	require.NoError(t, err)
	require.Equal(t, modelops.BackupJobStatusSuccess, job.Status)

	require.NoError(t, jobSvc.TriggerCleanup(ctx, "integration", "trace-cleanup"))

	drill, err := restoreSvc.Trigger(ctx, backupops.TriggerRestoreDrillRequest{SourceJobID: job.ID, Operator: "integration"})
	require.NoError(t, err)
	require.Equal(t, modelops.RestoreDrillStatusSuccess, drill.Status)
}

func setupBackupDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(&modelops.BackupPolicy{}, &modelops.BackupJob{}, &modelops.RestoreDrillRecord{}))
	return db
}
