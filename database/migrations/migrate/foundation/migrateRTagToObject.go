package foundation

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
)

type MigrateRTagToObject struct {
	*Migration
	MigrationInterface
}

func NewMigrateRTagToObject() *MigrateRTagToObject {
	return &MigrateRTagToObject{
		Migration: &Migration{
			Model: &tag.RTagToObject{},
		},
	}
}
