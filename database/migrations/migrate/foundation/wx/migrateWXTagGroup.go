package wx

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateWXTagGroup struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateWXTagGroup() *MigrateWXTagGroup {
	return &MigrateWXTagGroup{
		Migration: &migrate.Migration{
			Model: &models.WXTagGroup{},
		},
	}
}
