package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateUser struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateUser() *MigrateUser {
	return &MigrateUser{
		Migration: &migrate.Migration{
			Model: &models.User{},
		},
	}
}
