package foundation

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
)

type MigrateTag struct {
	*Migration
	MigrationInterface
}

func NewMigrateTag() *MigrateTag {
	return &MigrateTag{
		Migration: &Migration{
			Model: &tag.Tag{},
		},
	}
}
