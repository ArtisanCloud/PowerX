package wx

import (
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate/foundation"
)

type MigrateRWXTagToObject struct {
	*foundation.Migration
	foundation.MigrationInterface
}

func NewMigrateRWXTagToObject() *MigrateRWXTagToObject {
	return &MigrateRWXTagToObject{
		Migration: &foundation.Migration{
			Model: &wx.RWXTagToObject{},
		},
	}
}
