package seed

import (
	"PowerX/cmd/ctl/database/custom/seed"
	"PowerX/cmd/ctl/database/seed/datadictionary"
	"PowerX/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PowerSeeder struct {
	db   *gorm.DB
	conf *config.Config
}

func NewPowerSeeder(conf *config.Config) (*PowerSeeder, error) {
	var dsn gorm.Dialector
	switch conf.PowerXDatabase.Driver {
	case config.DriverMysql:
		dsn = mysql.Open(conf.PowerXDatabase.DSN)
	case config.DriverPostgres:
		dsn = postgres.Open(conf.PowerXDatabase.DSN)
	}
	db, err := gorm.Open(dsn, &gorm.Config{
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
	_ = datadictionary.CreateDataDictionaries(s.db)

	// 默认的组织架构是否需要生成
	if s.conf.PowerXDatabase.SeedDepartment {
		_ = CreateDefaultDepartments(s.db)
	}

	// 电商数据是否需要造数据
	if s.conf.PowerXDatabase.SeedCommerceData {
		_ = CreateMediaResources(s.db, s.conf)
		_ = CreateProductCategories(s.db)
		_ = CreateProducts(s.db)
		_ = CreateOrders(s.db)
		_ = CreatePayments(s.db)
	}

	_ = CreateTokenProducts(s.db)
	_ = CreatePriceBooks(s.db)
	_ = CreateTokenExchangeRatios(s.db)

	// custom
	seed.CreateCustomSeeds(s.db)

	return
}
