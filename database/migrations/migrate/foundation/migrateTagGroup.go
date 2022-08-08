package foundation

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
)

type MigrateTagGroup struct {
	*Migration
	MigrationInterface
}

func NewMigrateTagGroup() *MigrateTagGroup {
	return &MigrateTagGroup{
		Migration: &Migration{
			Model: &tag.TagGroup{},
		},
	}
}
