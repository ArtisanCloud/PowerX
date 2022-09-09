package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
)

type MigrateCustomerToEmployee struct {
	*Migration
	MigrationInterface
}

func NewMigrateCustomerToEmployee() *MigrateCustomerToEmployee {
	return &MigrateCustomerToEmployee{
		Migration: &Migration{
			Model: &models.RCustomerToEmployee{},
		},
	}
}
