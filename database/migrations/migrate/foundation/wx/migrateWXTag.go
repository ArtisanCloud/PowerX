package wx

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateWXTag struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateWXTag() *MigrateWXTag {
	return &MigrateWXTag{
		Migration: &migrate.Migration{
			Model: &models.WXTag{},
		},
	}
}
