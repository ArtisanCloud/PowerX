package foundation

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateTag struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateTag() *MigrateTag {
	return &MigrateTag{
		Migration: &migrate.Migration{
			Model: &tag.Tag{},
		},
	}
}
