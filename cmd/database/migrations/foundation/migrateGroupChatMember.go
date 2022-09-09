package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models/wx"
)

type MigrateGroupChatMember struct {
	*Migration
	MigrationInterface
}

func NewMigrateGroupChatMember() *MigrateGroupChatMember {
	return &MigrateGroupChatMember{
		Migration: &Migration{
			Model: &wx.WXGroupChatMember{},
		},
	}
}
