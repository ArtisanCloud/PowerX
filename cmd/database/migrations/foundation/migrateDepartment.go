package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
)

type MigrateDepartment struct {
	*Migration
	MigrationInterface
}

func NewMigrateDepartment() *MigrateDepartment {
	return &MigrateDepartment{
		Migration: &Migration{
			Model: &models.Department{},
		},
	}
}
