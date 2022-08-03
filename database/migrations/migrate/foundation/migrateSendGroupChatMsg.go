package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateSendGroupChatMsg struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateSendGroupChatMsg() *MigrateSendGroupChatMsg {
	return &MigrateSendGroupChatMsg{
		Migration: &migrate.Migration{
			Model: &models.SendGroupChatMsg{},
		},
	}
}
