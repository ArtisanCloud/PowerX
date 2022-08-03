package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateSendChatMsg struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateSendChatMsg() *MigrateSendChatMsg {
	return &MigrateSendChatMsg{
		Migration: &migrate.Migration{
			Model: &models.SendChatMsg{},
		},
	}
}
