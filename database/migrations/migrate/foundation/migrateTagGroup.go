package foundation

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateTagGroup struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateTagGroup() *MigrateTagGroup {
	return &MigrateTagGroup{
		Migration: &migrate.Migration{
			Model: &tag.TagGroup{},
		},
	}
}
