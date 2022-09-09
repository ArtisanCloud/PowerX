package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
)

type MigrateSendChatMsg struct {
	*Migration
	MigrationInterface
}

func NewMigrateSendChatMsg() *MigrateSendChatMsg {
	return &MigrateSendChatMsg{
		Migration: &Migration{
			Model: &models.SendChatMsg{},
		},
	}
}
