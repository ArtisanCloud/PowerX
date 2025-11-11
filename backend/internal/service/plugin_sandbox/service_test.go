package plugin_sandbox

import (
	"context"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_sandbox"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_sandbox"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSandboxFlow(t *testing.T) {
	db, cleanup := openSandboxDB(t)
	defer cleanup()

	runRepo := repo.NewRunRepository(db)
	suite := &Suite{
		Datasets: []DatasetSpec{
			{ID: "checkout-smoke", DefaultVersion: "2025-01"},
		},
	}
	suite.buildIndex()

	service := NewService(runRepo, Options{
		Suite: suite,
		Now:   func() time.Time { return time.Unix(0, 0) },
	})

	run, err := service.Deploy(context.Background(), DeployRequest{
		TenantID: 1001,
		PluginID: "plugin.demo",
		Dataset:  "checkout-smoke",
	})
	require.NoError(t, err)
	require.Equal(t, "deploying", run.Status)

	err = service.LoadDataset(context.Background(), DatasetRequest{
		RunID:     run.UUID,
		DatasetID: "checkout-smoke",
		Version:   "2025-01",
	})
	require.NoError(t, err)

	final, err := service.RunTests(context.Background(), TestRequest{
		RunID:    run.UUID,
		Outcome:  "passed",
		Metrics:  map[string]any{"coverage": 0.96},
		Report:   "s3://sandbox/report",
		Warnings: []string{},
	})
	require.NoError(t, err)
	require.Equal(t, "passed", final.Status)
	require.Equal(t, "2025-01", final.DatasetVersion)
	require.Equal(t, "s3://sandbox/report", final.ReportURI)
}

func openSandboxDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SandboxValidationRun{}))
	return db, func() {
		coremodel.PowerXSchema = previous
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}
