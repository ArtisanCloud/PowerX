package opsintegration

import (
	"context"
	"testing"

	deployops "github.com/ArtisanCloud/PowerX/internal/service/deploy_ops"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeployReleaseRollbackFlow(t *testing.T) {
	db := setupDeployDB(t)
	svc := deployops.NewService(db)
	ctx := context.Background()

	release, err := svc.TriggerRelease(ctx, deployops.ReleaseRequest{
		Environment:     "prod",
		BackendVersion:  "v1.2.3",
		WebAdminVersion: "v1.2.3",
		Mode:            deployops.DeployModeDocker,
		Operator:        "integration",
		TraceID:         "trace-us1-release",
		ApprovalTickets: 2,
	})
	require.NoError(t, err)
	require.Equal(t, modelops.DeployActionRelease, release.Action)
	require.Equal(t, modelops.DeployStatusSuccess, release.Status)

	rollback, err := svc.TriggerRollback(ctx, deployops.RollbackRequest{
		Environment:     "prod",
		TargetVersion:   "v1.2.2",
		Mode:            deployops.DeployModeDocker,
		Operator:        "integration",
		TraceID:         "trace-us1-rollback",
		ApprovalTickets: 2,
	})
	require.NoError(t, err)
	require.Equal(t, modelops.DeployActionRollback, rollback.Action)
	require.Equal(t, modelops.DeployStatusSuccess, rollback.Status)

	items, total, err := svc.ListReleases(ctx, deployops.ListReleaseOptions{Environment: "prod", Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, items, 2)

	health, err := svc.GetHealth(ctx)
	require.NoError(t, err)
	require.Equal(t, "healthy", health.Status)
}

func setupDeployDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&modelops.DeployReleaseRecord{},
		&modelops.ApprovalPolicyProfile{},
	))
	return db
}
