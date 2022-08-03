package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateDepartment struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateDepartment() *MigrateDepartment {
	return &MigrateDepartment{
		Migration: &migrate.Migration{
			Model: &models.WXDepartment{},
		},
	}
}
