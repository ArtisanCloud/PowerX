package foundation

import modelPowerLib "github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"

type MigrateRole struct {
	*Migration
	MigrationInterface
}

func NewMigrateRole() *MigrateRole {
	return &MigrateRole{
		Migration: &Migration{
			Model: &modelPowerLib.Role{},
		},
	}
}
