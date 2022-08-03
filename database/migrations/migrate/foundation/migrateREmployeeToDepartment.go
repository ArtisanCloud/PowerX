package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateREmployeeToDepartment struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateREmployeeToDepartment() *MigrateREmployeeToDepartment {
	return &MigrateREmployeeToDepartment{
		Migration: &migrate.Migration{
			Model: &models.REmployeeToDepartment{},
		},
	}
}
