package seed

import (
	"PowerX/cmd/ctl/database/custom/seed"
	"PowerX/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PowerSeeder struct {
	db   *gorm.DB
	conf *config.Config
}

func NewPowerSeeder(conf *config.Config) (*PowerSeeder, error) {
	db, err := gorm.Open(postgres.Open(conf.PowerXDatabase.DSN), &gorm.Config{
		//Logger:                                   logger.Default.LogMode(logger.Info),
		DisableForeignKeyConstraintWhenMigrating: true,
	})

	return &PowerSeeder{
		db:   db,
		conf: conf,
	}, err
}

func (s *PowerSeeder) CreatePowerX() (err error) {

	_ = CreateOrganization(s.db)
	_ = CreateDataDictionaries(s.db)
	_ = CreatePriceBooks(s.db)
	_ = CreateMediaResources(s.db, s.conf)
	_ = CreateProductCategories(s.db)
	_ = CreateProducts(s.db)

	// custom
	seed.CreateCustomSeeds(s.db)

	return
}
