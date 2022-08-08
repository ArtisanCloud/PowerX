package wx

import (
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate/foundation"
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
