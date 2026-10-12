package database

import (
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCustomerMenuMigrationPreservesExistingAuthorization(t *testing.T) {
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, db.AutoMigrate(&im.Permission{}, &im.RolePermission{}))
	require.NoError(t, EnsureCustomerMenuPermission(db))
	var row im.Permission
	require.NoError(t, db.Where("module = ? AND resource = ?", "menu", "customer").First(&row).Error)
	before := row.PermissionUUID
	require.False(t, row.AllowAPIKey)
	grant := im.RolePermission{RoleID: 7, RoleUUID: uuid.New(), PermissionID: row.ID, PermissionUUID: row.PermissionUUID}
	require.NoError(t, db.Create(&grant).Error)
	require.NoError(t, db.Model(&row).Updates(map[string]any{"effect": "deny", "status": "deprecated", "description": "operator setting"}).Error)
	require.NoError(t, EnsureCustomerMenuPermission(db))
	require.NoError(t, db.First(&row, row.ID).Error)
	require.Equal(t, before, row.PermissionUUID)
	require.Equal(t, "deny", row.Effect)
	require.Equal(t, im.PermissionStatusDeprecated, row.Status)
	require.Equal(t, "operator setting", row.Description)
	var count int64
	require.NoError(t, db.Model(&im.RolePermission{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Delete(&row).Error)
	require.NoError(t, EnsureCustomerMenuPermission(db))
	require.NoError(t, db.Unscoped().First(&row, row.ID).Error)
	require.True(t, row.DeletedAt.Valid)
}
