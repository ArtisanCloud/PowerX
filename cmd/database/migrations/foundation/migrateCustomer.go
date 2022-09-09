package foundation

import (
	"github.com/ArtisanCloud/PowerX/app/models"
)

type MigrateCustomer struct {
	*Migration
	MigrationInterface
}

func NewMigrateCustomer() *MigrateCustomer {
	return &MigrateCustomer{
		Migration: &Migration{
			Model: &models.Customer{},
		},
	}
}

//func(m *MigrateCustomer) Migrate() error {
//
//	fmt.Dump("migrate customer .......")
//
//	return nil
//}
