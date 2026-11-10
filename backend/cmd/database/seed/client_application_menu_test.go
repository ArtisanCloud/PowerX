package seed

import (
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestClientApplicationMenuSeedPreservesExistingPolicy(t *testing.T) {
	db := roleIdentityDB(t)
	require.NoError(t, SeedClientApplicationMenu(db))
	var permission im.Permission
	require.NoError(t, db.Where("module = ? AND resource = ?", "menu", "settings.client_apps").First(&permission).Error)
	require.False(t, permission.AllowAPIKey)
	require.NoError(t, db.Model(&permission).Updates(map[string]any{"effect": "deny", "status": "deprecated"}).Error)
	require.NoError(t, SeedClientApplicationMenu(db))
	require.NoError(t, db.First(&permission, permission.ID).Error)
	require.Equal(t, "deny", permission.Effect)
	require.EqualValues(t, "deprecated", permission.Status)
	var count int64
	require.NoError(t, db.Model(&im.RolePermission{}).Count(&count).Error)
	require.Zero(t, count, "menu preparation must not grant roles")
}
