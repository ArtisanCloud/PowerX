package foundation

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/boostrap"
	"github.com/ArtisanCloud/PowerX/boostrap/global"
	"gorm.io/gorm"
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

func (m *Migration) Migrate(db *gorm.DB) error {

	hasTable := db.Migrator().HasTable(m.Model)

	m.TableName = m.GetTableName()

	// force drop tables if it has this table
	if NeedRefresh && hasTable {
		err := global.DBConnection.Migrator().DropTable(m.Model)
		if err != nil {
			println(err.Error())
			return err
		} else {
			hasTable = false
			fmt.Printf("has dropped table:%s \n", m.TableName)
		}
	}

	if !hasTable {
		err := global.DBConnection.Migrator().CreateTable(m.Model)
		//err := global.DBConnection.Migrator().AutoMigrate(table)
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
