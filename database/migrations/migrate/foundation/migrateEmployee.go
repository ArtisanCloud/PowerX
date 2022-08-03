package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateEmployee struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateEmployee() *MigrateEmployee {
	return &MigrateEmployee{
		Migration: &migrate.Migration{
			Model: &models.Employee{},
		},
	}
}
