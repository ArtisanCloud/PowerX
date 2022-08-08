package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
)

type MigrateSendGroupChatMsg struct {
	*Migration
	MigrationInterface
}

func NewMigrateSendGroupChatMsg() *MigrateSendGroupChatMsg {
	return &MigrateSendGroupChatMsg{
		Migration: &Migration{
			Model: &models.SendGroupChatMsg{},
		},
	}
}
