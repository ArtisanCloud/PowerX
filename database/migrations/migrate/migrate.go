package migrate

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/boostrap"
	"github.com/ArtisanCloud/PowerX/database"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate/foundation"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate/foundation/wx"
	"reflect"
)

var (
	NeedRefresh bool
	Industry    string
)

func init() {
	boostrap.InitProject()
}

type MigrationInterface interface {
	GetModel() interface{}
	GetTableName() string
}

type Migration struct {
	Model     interface{}
	TableName string
}

func (m *Migration) Run() (err error) {

	if err != nil {
		return err
	}
	// Power tables
	err = foundation.NewMigratePowerOperationLog().Migrate()

	// marketX tables
	err = foundation.NewMigrateAccount().Migrate()
	//err = NewMigrateCommission().Migrate()
	//err = NewMigrateCoupon().Migrate()
	//err = NewMigrateCouponItem().Migrate()
	err = foundation.NewMigrateDepartment().Migrate()
	err = foundation.NewMigrateEmployee().Migrate()
	//err = NewMigrateFission().Migrate()
	//err = NewMigrateFissionLog().Migrate()
	//err = NewMigrateRFissionToAward().Migrate()
	//err = NewMigrateRFissionToCampaign().Migrate()
	//err = NewMigrateRFissionToExperience().Migrate()
	//err = NewMigrateMembership().Migrate()
	//err = NewMigrateNotification().Migrate()
	//err = NewMigrateMerchant().Migrate()
	//err = NewMigrateOrder().Migrate()
	//err = NewMigrateOrderItem().Migrate()
	//err = NewMigratePayment().Migrate()
	//err = NewMigratePicklistOption().Migrate()
	//err = NewMigratePriceConfig().Migrate()
	//err = NewMigrateProduct().Migrate()
	//err = NewMigratePriceBookEntry().Migrate()
	//err = NewMigratePriceBook().Migrate()
	//err = NewMigrateReseller().Migrate()
	//err = NewMigrateRCouponToProduct().Migrate()
	err = foundation.NewMigrateREmployeeToDepartment().Migrate()
	err = foundation.NewMigrateUser().Migrate()
	err = foundation.NewMigrateTag().Migrate()
	err = foundation.NewMigrateTagGroup().Migrate()
	err = foundation.NewMigrateRTagToObject().Migrate()
	err = foundation.NewMigrateContactWay().Migrate()
	err = foundation.NewMigrateContactWayGroup().Migrate()
	err = foundation.NewMigrateGroupChat().Migrate()
	err = foundation.NewMigrateGroupChatMember().Migrate()
	err = foundation.NewMigrateGroupChatAdmin().Migrate()
	err = foundation.NewMigrateSendChatMsg().Migrate()
	err = foundation.NewMigrateSendGroupChatMsg().Migrate()

	// wx tables
	err = foundation.NewMigrateCustomerToEmployee().Migrate()
	err = wx.NewMigrateWXTag().Migrate()
	err = foundation.NewMigrateRWXTagToObject().Migrate()
	err = wx.NewMigrateWXMessageTemplate().Migrate()
	err = wx.NewMigrateWXMessageTemplateTask().Migrate()
	err = wx.NewMigrateWXMessageTemplateSend().Migrate()
	return err
}

func (m *Migration) Migrate() error {

	hasTable := database.DBConnection.Migrator().HasTable(m.Model)

	m.TableName = m.GetTableName()

	// force drop tables if it has this table
	if NeedRefresh && hasTable {
		err := database.DBConnection.Migrator().DropTable(m.Model)
		if err != nil {
			println(err.Error())
			return err
		} else {
			hasTable = false
			fmt.Printf("has dropped table:%s \n", m.TableName)
		}
	}

	if !hasTable {
		err := database.DBConnection.Migrator().CreateTable(m.Model)
		//err := database.DBConnection.Migrator().AutoMigrate(table)
		if err != nil {
			println("create table error: ", err.Error())
			return err
		} else {
			fmt.Printf("has created table:%s \n\n", m.TableName)
		}

	}
	return nil
}

func (m *Migration) GetModel() interface{} {
	return m.Model
}

func (m *Migration) GetTableName() string {

	tableName := reflect.TypeOf(m.Model).String()

	return tableName
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
