package wx

import (
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/cmd/database/migrations/foundation"
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
