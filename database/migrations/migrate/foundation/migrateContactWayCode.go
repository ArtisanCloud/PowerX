package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
)

type MigrateContactWay struct {
	*Migration
	MigrationInterface
}

func NewMigrateContactWay() *MigrateContactWay {
	return &MigrateContactWay{
		Migration: &Migration{
			Model: &models.ContactWay{},
		},
	}
}
