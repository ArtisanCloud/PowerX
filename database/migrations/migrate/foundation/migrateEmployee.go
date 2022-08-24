package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
)

type MigrateEmployee struct {
	*Migration
	MigrationInterface
}

func NewMigrateEmployee() *MigrateEmployee {
	return &MigrateEmployee{
		Migration: &Migration{
			Model: &models.Employee{},
		},
	}
}
