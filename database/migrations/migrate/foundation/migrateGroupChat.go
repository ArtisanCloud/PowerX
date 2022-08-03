package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateGroupChat struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateGroupChat() *MigrateGroupChat {
	return &MigrateGroupChat{
		Migration: &migrate.Migration{
			Model: &models.GroupChat{},
		},
	}
}
