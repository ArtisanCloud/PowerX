package plugin_compat

import (
	"context"
	"fmt"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	compatmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_compat"
	compatrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_compat"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCompatService(t *testing.T) {
	db := openCompatDB(t)
	repo := compatrepo.NewExceptionRepository(db)
	service := NewService(repo, func() time.Time { return time.Unix(0, 0) })

	res, err := service.Check(context.Background(), CheckRequest{HostVersion: "1.2.0", PluginVersion: "1.5.0"})
	require.NoError(t, err)
	require.False(t, res.Compatible)

	exception, err := service.CreateException(context.Background(), ExceptionRequest{
		TenantUUID:     "tenant",
		PluginID:       "plugin",
		CurrentVersion: "1.0.0",
		TargetVersion:  "2.0.0",
		Reason:         "legacy",
	})
	require.NoError(t, err)

	updated, err := service.Approve(context.Background(), ApproveRequest{
		ID:       exception.UUID,
		Status:   "approved",
		Reviewer: "admin",
	})
	require.NoError(t, err)
	require.Equal(t, "approved", updated.Status)
}

func openCompatDB(t *testing.T) *gorm.DB {
	t.Helper()
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	dsn := fmt.Sprintf("file:compat-tests-%d?mode=memory&cache=shared&_loc=UTC", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&compatmodel.CompatException{}))
	return db
}
