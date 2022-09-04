package foundation

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/boostrap"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database/global"
	"gorm.io/gorm"
	"reflect"
)

var (
	NeedRefresh bool
	Industry    string
)

func init() {
	var err error

	err = boostrap.InitConfig()
	if err != nil {
		panic(err)
	}

	// 模拟系统已经安装成功
	config.G_AppConfigure.SystemConfig.Installed = true

	err = boostrap.InitProject()
	if err != nil {
		panic(err)
	}
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
		err := global.G_DBConnection.Migrator().DropTable(m.Model)
		if err != nil {
			println(err.Error())
			return err
		} else {
			hasTable = false
			fmt.Printf("has dropped table:%s \n", m.TableName)
		}
	}

	if !hasTable {
		err := global.G_DBConnection.Migrator().CreateTable(m.Model)
		//err := global.G_DBConnection.Migrator().AutoMigrate(table)
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
