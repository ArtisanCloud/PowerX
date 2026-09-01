package seed

import (
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCleanupInvalidPermissionRowsDeletesInvalidRowsAndBindings(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = prevSchema
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&dbm.Permission{}, &dbm.RolePermission{}, &dbm.APIKeyProfilePermission{}))

	valid := dbm.Permission{
		Module: "iam", Resource: "role", Action: "read", Effect: "allow", Status: dbm.PermissionStatusActive,
		Source: "platform_capability",
		Meta:   []byte(`{"type":"api","title_i18n":{"zh-CN":"角色读取"}}`),
	}
	candidate := dbm.Permission{
		Module: "admin", Resource: "raw", Action: "read", Effect: "allow", Status: dbm.PermissionStatusDeprecated,
		Source: "platform_capability_generated",
		Meta:   []byte(`{"type":"api_candidate"}`),
	}
	legacyPlugin := dbm.Permission{
		Module: "com.powerx.plugins.base.local", Resource: "template", Action: "read", Effect: "allow", Status: dbm.PermissionStatusActive,
		Source: "com.powerx.plugins.base.local",
		Meta:   []byte(`{"type":"action","title_i18n":{"zh-CN":"读取模板"}}`),
	}
	unnamedAPI := dbm.Permission{
		Module: "admin", Resource: "unnamed", Action: "read", Effect: "allow", Status: dbm.PermissionStatusActive,
		Source: "platform_capability",
		Meta:   []byte(`{"type":"api"}`),
	}
	require.NoError(t, db.Create(&valid).Error)
	require.NoError(t, db.Create(&candidate).Error)
	require.NoError(t, db.Create(&legacyPlugin).Error)
	require.NoError(t, db.Create(&unnamedAPI).Error)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&dbm.RolePermission{RoleID: 1, PermissionID: candidate.ID, RoleUUID: uuid.New(), PermissionUUID: uuid.New()}).Error)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&dbm.RolePermission{RoleID: 1, PermissionID: valid.ID, RoleUUID: uuid.New(), PermissionUUID: uuid.New()}).Error)
	require.NoError(t, db.Create(&dbm.APIKeyProfilePermission{ProfileID: 1, PermissionID: unnamedAPI.ID}).Error)

	require.NoError(t, CleanupInvalidPermissionRows(db))

	var remaining []dbm.Permission
	require.NoError(t, db.Order("id ASC").Find(&remaining).Error)
	require.Len(t, remaining, 1)
	require.Equal(t, valid.ID, remaining[0].ID)

	var roleBindingCount int64
	require.NoError(t, db.Model(&dbm.RolePermission{}).Count(&roleBindingCount).Error)
	require.Equal(t, int64(1), roleBindingCount)
	var apiKeyBindingCount int64
	require.NoError(t, db.Model(&dbm.APIKeyProfilePermission{}).Count(&apiKeyBindingCount).Error)
	require.Equal(t, int64(0), apiKeyBindingCount)
	require.NoError(t, EnsureSeededPermissionRowsValid(db))
}

func TestEnsureSeededPermissionRowsValidRejectsActiveInvalidRows(t *testing.T) {
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = prevSchema
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&dbm.Permission{}))

	require.NoError(t, db.Create(&dbm.Permission{
		Module: "admin", Resource: "unnamed", Action: "read", Effect: "allow", Status: dbm.PermissionStatusActive,
		Source: "platform_capability",
		Meta:   []byte(`{"type":"api"}`),
	}).Error)

	err = EnsureSeededPermissionRowsValid(db)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid_active_apis=1")
}
