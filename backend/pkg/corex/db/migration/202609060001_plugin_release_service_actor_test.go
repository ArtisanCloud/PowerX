package migration

import (
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsurePluginReleaseServiceActorMigrationRejectsUnmappableLegacyRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	require.NoError(t, db.Exec(`CREATE TABLE plugin_release_local_install_sessions (id integer primary key, tenant_uuid text, status text)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO plugin_release_local_install_sessions (tenant_uuid, status) VALUES ('tenant', 'in_progress')`).Error)

	err = EnsurePluginReleaseServiceActorMigration(db)
	require.ErrorContains(t, err, "lack plugin_id/service_actor")
}
