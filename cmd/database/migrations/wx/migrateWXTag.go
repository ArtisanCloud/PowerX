package wx

import (
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/cmd/database/migrations/foundation"
)

type MigrateWXTag struct {
	*foundation.Migration
	foundation.MigrationInterface
}

func NewMigrateWXTag() *MigrateWXTag {
	return &MigrateWXTag{
		Migration: &foundation.Migration{
			Model: &wx.WXTag{},
		},
	}
}
