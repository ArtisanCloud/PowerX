package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateContactWayGroup struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateContactWayGroup() *MigrateContactWayGroup {
	return &MigrateContactWayGroup{
		Migration: &migrate.Migration{
			Model: &models.ContactWayGroup{},
		},
	}
}
