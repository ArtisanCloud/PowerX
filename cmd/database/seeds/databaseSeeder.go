package main

import (
	"github.com/ArtisanCloud/PowerX/boostrap"
	"github.com/ArtisanCloud/PowerX/config"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"os"
)

func init() {

	err := boostrap.InitConfig()
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

type SeederInterface interface {
	Run(ctx *gin.Context) error
}

type DatabaseSeeder struct {
	SeederInterface
}

func NewDatabaseSeeder(ctx *gin.Context) *DatabaseSeeder {
	return &DatabaseSeeder{}
}

func (seeder *DatabaseSeeder) Run(ctx *gin.Context) (err error) {

	err = globalDatabase.G_DBConnection.Transaction(func(tx *gorm.DB) error {
		// run seeders here
		err = NewTagTableSeeder(ctx).Run(ctx)
		//err = NewUserTableSeeder(ctx).Run(ctx)
		//err = NewMerchantTableSeeder(ctx).Run(ctx)
		//err = NewCouponTableSeeder(ctx).Run(ctx)
		//err = NewPriceBookTableSeeder(ctx).Run(ctx)
		//err = NewProductTableSeeder(ctx).Run(ctx)
		//err = NewCustomerTableSeeder(ctx).Run(ctx)
		//err = NewResellerTableSeeder(ctx).Run(ctx)
		//err = NewOrderTableSeeder(ctx).Run(ctx)
		err = NewWXContactWayGroupTableSeeder(ctx).Run(ctx)

		return err
	})
	return err
}

func main() {

	ctx := &gin.Context{}

	dbSeeder := NewDatabaseSeeder(ctx)

	err := dbSeeder.Run(ctx)
	if err != nil {
		println(err.Error())
		os.Exit(-1)
	}

	println("seed done")

	return
}