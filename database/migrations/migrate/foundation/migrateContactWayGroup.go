package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
)

type MigrateContactWayGroup struct {
	*Migration
	MigrationInterface
}

func NewMigrateContactWayGroup() *MigrateContactWayGroup {
	return &MigrateContactWayGroup{
		Migration: &Migration{
			Model: &models.ContactWayGroup{},
		},
	}
}
