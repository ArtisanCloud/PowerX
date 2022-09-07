package boostrap

import (
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/app/service/wx/miniProgram"
	"github.com/ArtisanCloud/PowerX/app/service/wx/weCom"
	cache2 "github.com/ArtisanCloud/PowerX/boostrap/cache"
	"github.com/ArtisanCloud/PowerX/boostrap/rbac"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database"
	"github.com/ArtisanCloud/PowerX/database/global"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/ArtisanCloud/PowerX/resources/lang"
	"github.com/gin-gonic/gin"
)

func InitConfig() (err error) {

	// Initialize the global config
	envConfigPath := "config.yml"

	err = config.LoadEnvConfig(&envConfigPath)

	return err
}

func InitProject() (err error) {

	// Initialize the logger
	err = logger.SetupLog(&config.G_AppConfigure.LogConfig)
	if err != nil {
		return err
	}

	err = config.LoadDatabaseConfig()
	if err != nil {
		return err
	}

	err = config.LoadCacheConfig()
	if err != nil {
		return err
	}

	config.LoadVersion()

	// load locale
	lang.LoadLanguages()

	// setup ssh key path
	err = service.SetupJWTKeyPairs(&config.G_AppConfigure.JWTConfig)
	if err != nil {
		return err
	}

	// Initialize the cache
	err = cache2.SetupCache(&config.G_AppConfigure.CacheConfig.CacheConnections.RedisConfig)
	if err != nil {
		return err
	}

	// Initialize the database
	err = database.SetupDatabase(config.G_DBConfig)
	if err != nil {
		return err
	}

	// Initialize the RBAC Enforcer
	err = rbac.InitCasbin(global.G_DBConnection)
	if err != nil {
		panic(err)
	}

	err = InitServices()
	if err != nil {
		return err
	}

	return err
}

func InitServices() (err error) {

	// defined singleton located in app/service/wechat/weCom/datetime.go
	if weCom.G_WeComApp == nil {
		weCom.G_WeComApp, err = weCom.NewWeComService(nil, &config.G_AppConfigure.WecomConfig)
		if err != nil {
			return err
		}
	}

	// defined singleton located in app/service/wechat/weCom/datetime.go
	if weCom.G_WeComEmployee == nil {
		ctx := &gin.Context{}
		ctx.Set("messageToken", config.G_AppConfigure.WecomConfig.EmployeeMessageToken)
		ctx.Set("messageAESKey", config.G_AppConfigure.WecomConfig.EmployeeMessageAesKey)
		ctx.Set("messageCallbackURL", config.G_AppConfigure.WecomConfig.EmployeeMessageCallbackURL)
		weCom.G_WeComEmployee, err = weCom.NewWeComService(ctx, &config.G_AppConfigure.WecomConfig)
		if err != nil {
			return err
		}
	}

	// defined singleton located in app/service/wechat/weCom/datetime.go
	if weCom.G_WeComCustomer == nil {
		ctx := &gin.Context{}
		ctx.Set("messageToken", config.G_AppConfigure.WecomConfig.CustomerMessageToken)
		ctx.Set("messageAESKey", config.G_AppConfigure.WecomConfig.CustomerMessageAesKey)
		ctx.Set("messageCallbackURL", config.G_AppConfigure.WecomConfig.CustomerMessageCallbackURL)
		weCom.G_WeComCustomer, err = weCom.NewWeComService(ctx, &config.G_AppConfigure.WecomConfig)
		if err != nil {
			return err
		}
	}

	// defined singleton located in app/service/wechat/miniprogram/datetime.go
	if miniProgram.MiniProgramApp == nil {
		miniProgram.MiniProgramApp, err = miniProgram.NewMiniProgramService(nil)
		if err != nil {
			return err
		}
	}

	//if global.PaymentApp == nil {
	//	global.PaymentApp, err = weCom.NewPaymentService(nil)
	//	if err != nil {
	//		return err
	//	}
	//}

	return err

}
