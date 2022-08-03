package education

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateClass struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateClass() *MigrateClass {
	return &MigrateClass{
		Migration: &migrate.Migration{
			Model: &models.Class{},
		},
	}
}
