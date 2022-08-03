package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateCustomerToEmployee struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateCustomerToEmployee() *MigrateCustomerToEmployee {
	return &MigrateCustomerToEmployee{
		Migration: &migrate.Migration{
			Model: &models.RCustomerToEmployee{},
		},
	}
}
