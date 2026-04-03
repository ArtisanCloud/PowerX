package opsintegration

import (
	"context"
	"testing"

	migrationops "github.com/ArtisanCloud/PowerX/internal/service/migration_ops"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestInstanceMigrationFlow(t *testing.T) {
	db := setupMigrationDB(t)
	svc := migrationops.NewService(db)
	ctx := context.Background()

	record, err := svc.TriggerMigration(ctx, migrationops.TriggerRequest{
		SourceEnv: "prod-a",
		TargetEnv: "prod-b",
		DryRun:    false,
		Operator:  "integration",
		TraceID:   "trace-migration-trigger",
	})
	require.NoError(t, err)
	require.Equal(t, modelops.MigrationStepSuccess, record.DBMigrationStatus)
	require.Equal(t, modelops.MigrationStepPending, record.InstanceAcceptanceStatus)

	record, err = svc.AcceptMigration(ctx, migrationops.AcceptanceRequest{
		MigrationID:              record.ID,
		DBMigrationCompleted:     true,
		InstanceMigrationPassed:  true,
		AcceptanceConclusionNote: "core business checks passed",
	})
	require.NoError(t, err)
	require.Equal(t, modelops.MigrationStatusSuccess, record.Status)

	opSwitch, switchedRecord, err := svc.TriggerTrafficSwitch(ctx, migrationops.SwitchRequest{
		MigrationID: record.ID,
		Rollback:    false,
		Operator:    "integration",
		TraceID:     "trace-switch",
	})
	require.NoError(t, err)
	require.NotEmpty(t, opSwitch)
	require.Equal(t, modelops.MigrationStepSuccess, switchedRecord.TrafficSwitchStatus)

	opRollback, rollbackRecord, err := svc.TriggerTrafficSwitch(ctx, migrationops.SwitchRequest{
		MigrationID: record.ID,
		Rollback:    true,
		Operator:    "integration",
		TraceID:     "trace-rollback",
	})
	require.NoError(t, err)
	require.NotEmpty(t, opRollback)
	require.Equal(t, modelops.MigrationStepSuccess, rollbackRecord.TrafficRollbackStatus)
}

func setupMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(&modelops.MigrationRunbookRecord{}))
	return db
}
