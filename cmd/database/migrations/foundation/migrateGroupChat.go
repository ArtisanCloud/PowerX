package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
)

type MigrateGroupChat struct {
	*Migration
	MigrationInterface
}

func NewMigrateGroupChat() *MigrateGroupChat {
	return &MigrateGroupChat{
		Migration: &Migration{
			Model: &models.GroupChat{},
		},
	}
}
