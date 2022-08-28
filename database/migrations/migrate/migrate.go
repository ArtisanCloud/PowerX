package migrate

import (
	"github.com/ArtisanCloud/PowerX/boostrap"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate/foundation"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate/wx"
	"gorm.io/gorm"
)

var (
	NeedRefresh bool
	//Industry    string
)

func init() {
	err := boostrap.InitProject()
	if err != nil {
		panic(err)
	}
}

func Run(db *gorm.DB) (err error) {

	if err != nil {
		return err
	}
	// Power tables
	err = foundation.NewMigratePowerOperationLog().Migrate(db)

	// marketX tables
	err = foundation.NewMigrateCustomer().Migrate(db)
	err = foundation.NewMigrateDepartment().Migrate(db)
	err = foundation.NewMigrateEmployee().Migrate(db)
	err = foundation.NewMigrateRole().Migrate(db)
	err = foundation.NewMigratePermissionModule().Migrate(db)
	err = foundation.NewMigratePermission().Migrate(db)
	err = foundation.NewMigrateREmployeeToDepartment().Migrate(db)
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
	err = foundation.NewMigrateTag().Migrate(db)
	err = foundation.NewMigrateTagGroup().Migrate(db)
	err = foundation.NewMigrateRTagToObject().Migrate(db)
	err = foundation.NewMigrateContactWay().Migrate(db)
	err = foundation.NewMigrateContactWayGroup().Migrate(db)
	err = foundation.NewMigrateGroupChat().Migrate(db)
	err = foundation.NewMigrateGroupChatMember().Migrate(db)
	err = foundation.NewMigrateGroupChatAdmin().Migrate(db)
	err = foundation.NewMigrateSendChatMsg().Migrate(db)
	err = foundation.NewMigrateSendGroupChatMsg().Migrate(db)
	err = foundation.NewMigrateCustomerToEmployee().Migrate(db)

	// wechat tables
	err = wx.NewMigrateWXTag().Migrate(db)
	err = wx.NewMigrateRWXTagToObject().Migrate(db)
	err = wx.NewMigrateWXMessageTemplate().Migrate(db)
	err = wx.NewMigrateWXMessageTemplateTask().Migrate(db)
	err = wx.NewMigrateWXMessageTemplateSend().Migrate(db)
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
