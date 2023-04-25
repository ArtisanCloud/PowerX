package seed

import (
<<<<<<< HEAD
	"PowerX/deploy/database/custom/seed"
=======
	"PowerX/deploy/database/cusotm/seed"
	"PowerX/internal/config"
	"gorm.io/driver/postgres"
>>>>>>> powerx/develop
	"gorm.io/gorm"
)

type PowerSeeder struct {
	db *gorm.DB
}

func NewPowerSeeder(conf *config.Config) (*PowerSeeder, error) {
	db, err := gorm.Open(postgres.Open(conf.PowerXDatabase.DSN), &gorm.Config{
		//Logger:                                   logger.Default.LogMode(logger.Info),
		DisableForeignKeyConstraintWhenMigrating: true,
	})

	return &PowerSeeder{
		db: db,
	}, err
}

func (s *PowerSeeder) CreatePowerX() (err error) {

	_ = CreateOrganization(s.db)
	_ = CreateDataDictionaries(s.db)
	_ = CreatePriceBooks(s.db)
	_ = CreateProductCategories(s.db)

	// custom
	seed.CreateCustomSeeds(s.db)

	// Custom
	_ = seed.CreateStore(db)

	return
}
