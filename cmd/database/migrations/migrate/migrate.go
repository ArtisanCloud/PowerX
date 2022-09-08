package migrate

import (
	"github.com/ArtisanCloud/PowerX/boostrap"
	foundation2 "github.com/ArtisanCloud/PowerX/cmd/database/migrations/migrate/foundation"
	wx2 "github.com/ArtisanCloud/PowerX/cmd/database/migrations/migrate/wx"
	"github.com/ArtisanCloud/PowerX/config"
	database2 "github.com/ArtisanCloud/PowerX/database"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"gorm.io/gorm"
)

var (
	NeedRefresh bool
	//Industry    string
)

func init() {
	var err error
	err = boostrap.InitConfig()
	if err != nil {
		panic(err)
	}

	// Initialize the logger
	err = logger.SetupLog(&config.G_AppConfigure.LogConfig)
	if err != nil {
		panic(err)
	}

	err = config.LoadDatabaseConfig()
	if err != nil {
		panic(err)
	}

	// Initialize the database
	err = database2.SetupDatabase(config.G_DBConfig)
	if err != nil {
		panic(err)
	}

}

func Run(db *gorm.DB) (err error) {

	if err != nil {
		return err
	}
	// Power tables
	err = foundation2.NewMigratePowerOperationLog().Migrate(db)

	// marketX tables
	err = foundation2.NewMigrateCustomer().Migrate(db)
	err = foundation2.NewMigrateDepartment().Migrate(db)
	err = foundation2.NewMigrateEmployee().Migrate(db)
	err = foundation2.NewMigrateRole().Migrate(db)
	err = foundation2.NewMigratePermissionModule().Migrate(db)
	err = foundation2.NewMigratePermission().Migrate(db)
	err = foundation2.NewMigrateREmployeeToDepartment().Migrate(db)
	//err = migrate.NewMigrateCommission().Migrate(db)
	//err = migrate.NewMigrateCoupon().Migrate(db)
	//err = migrate.NewMigrateCouponItem().Migrate(db)
	//err = migrate.NewMigrateFission().Migrate(db)
	//err = migrate.NewMigrateFissionLog().Migrate(db)
	//err = migrate.NewMigrateRFissionToAward().Migrate(db)
	//err = migrate.NewMigrateRFissionToCampaign().Migrate(db)
	//err = migrate.NewMigrateRFissionToExperience().Migrate(db)
	//err = migrate.NewMigrateMembership().Migrate(db)
	//err = migrate.NewMigrateNotification().Migrate(db)
	//err = migrate.NewMigrateMerchant().Migrate(db)
	//err = migrate.NewMigrateOrder().Migrate(db)
	//err = migrate.NewMigrateOrderItem().Migrate(db)
	//err = migrate.NewMigratePayment().Migrate(db)
	//err = migrate.NewMigratePicklistOption().Migrate(db)
	//err = migrate.NewMigratePriceConfig().Migrate(db)
	//err = migrate.NewMigrateProduct().Migrate(db)
	//err = migrate.NewMigratePriceBookEntry().Migrate(db)
	//err = migrate.NewMigratePriceBook().Migrate(db)
	//err = migrate.NewMigrateReseller().Migrate(db)
	//err = migrate.NewMigrateRCouponToProduct().Migrate(db)
	err = foundation2.NewMigrateTag().Migrate(db)
	err = foundation2.NewMigrateTagGroup().Migrate(db)
	err = foundation2.NewMigrateRTagToObject().Migrate(db)
	err = foundation2.NewMigrateContactWay().Migrate(db)
	err = foundation2.NewMigrateContactWayGroup().Migrate(db)
	err = foundation2.NewMigrateGroupChat().Migrate(db)
	err = foundation2.NewMigrateGroupChatMember().Migrate(db)
	err = foundation2.NewMigrateGroupChatAdmin().Migrate(db)
	err = foundation2.NewMigrateSendChatMsg().Migrate(db)
	err = foundation2.NewMigrateSendGroupChatMsg().Migrate(db)
	err = foundation2.NewMigrateCustomerToEmployee().Migrate(db)

	// wechat tables
	err = wx2.NewMigrateWXTag().Migrate(db)
	err = wx2.NewMigrateRWXTagToObject().Migrate(db)
	err = wx2.NewMigrateWXMessageTemplate().Migrate(db)
	err = wx2.NewMigrateWXMessageTemplateTask().Migrate(db)
	err = wx2.NewMigrateWXMessageTemplateSend().Migrate(db)
	return err
}

func appendIndustryTables(industry string, arrayTables []interface{}) []interface{} {
	arrayIndustryTables := []interface{}{
		//&models.Class{},
		//&models.Classroom{},
		//&models.Reservation{},
		//&models.Schedule{},
		//&models.School{},
	}
	for _, table := range arrayIndustryTables {
		arrayTables = append(arrayTables, table)
	}

	return arrayTables
}
