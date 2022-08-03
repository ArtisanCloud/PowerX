package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateGroupChatAdmin struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateGroupChatAdmin() *MigrateGroupChatAdmin {
	return &MigrateGroupChatAdmin{
		Migration: &migrate.Migration{
			Model: &models.WXGroupChatAdmin{},
		},
	}
}
