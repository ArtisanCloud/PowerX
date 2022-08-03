package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateGroupChatMember struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateGroupChatMember() *MigrateGroupChatMember {
	return &MigrateGroupChatMember{
		Migration: &migrate.Migration{
			Model: &models.WXGroupChatMember{},
		},
	}
}
