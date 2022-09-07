package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models/wx"
)

type MigrateGroupChatAdmin struct {
	*Migration
	MigrationInterface
}

func NewMigrateGroupChatAdmin() *MigrateGroupChatAdmin {
	return &MigrateGroupChatAdmin{
		Migration: &Migration{
			Model: &wx.WXGroupChatAdmin{},
		},
	}
}
