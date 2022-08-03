package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateAccount struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateAccount() *MigrateAccount {
	return &MigrateAccount{
		Migration: &migrate.Migration{
			Model: &models.Account{},
		},
	}
}

//func(m *MigrateAccount) Migrate() error {
//
//	fmt.Dump("migrate customer .......")
//
//	return nil
//}
