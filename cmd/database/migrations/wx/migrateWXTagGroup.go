package wx

import (
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/cmd/database/migrations/foundation"
)

type MigrateWXTagGroup struct {
	*foundation.Migration
	foundation.MigrationInterface
}

func NewMigrateWXTagGroup() *MigrateWXTagGroup {
	return &MigrateWXTagGroup{
		Migration: &foundation.Migration{
			Model: &wx.WXTagGroup{},
		},
	}
}
