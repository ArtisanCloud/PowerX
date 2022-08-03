package foundation

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateRTagToObject struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateRTagToObject() *MigrateRTagToObject {
	return &MigrateRTagToObject{
		Migration: &migrate.Migration{
			Model: &tag.RTagToObject{},
		},
	}
}
