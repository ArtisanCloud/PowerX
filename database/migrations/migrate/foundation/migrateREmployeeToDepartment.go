package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
)

type MigrateREmployeeToDepartment struct {
	*Migration
	MigrationInterface
}

func NewMigrateREmployeeToDepartment() *MigrateREmployeeToDepartment {
	return &MigrateREmployeeToDepartment{
		Migration: &Migration{
			Model: &models.REmployeeToDepartment{},
		},
	}
}
