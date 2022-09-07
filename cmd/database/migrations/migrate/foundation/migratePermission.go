package foundation

import modelPowerLib "github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"

type MigratePermission struct {
	*Migration
	MigrationInterface
}

func NewMigratePermission() *MigratePermission {
	return &MigratePermission{
		Migration: &Migration{
			Model: &modelPowerLib.Permission{},
		},
	}
}

type MigratePermissionModule struct {
	*Migration
	MigrationInterface
}

func NewMigratePermissionModule() *MigratePermissionModule {
	return &MigratePermissionModule{
		Migration: &Migration{
			Model: &modelPowerLib.PermissionModule{},
		},
	}
}
