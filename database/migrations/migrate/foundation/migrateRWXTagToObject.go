package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateRWXTagToObject struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateRWXTagToObject() *MigrateRWXTagToObject {
	return &MigrateRWXTagToObject{
		Migration: &migrate.Migration{
			Model: &models.RWXTagToObject{},
		},
	}
}
