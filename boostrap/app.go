package boostrap

import (
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/app/service/wx/miniProgram"
	"github.com/ArtisanCloud/PowerX/app/service/wx/wecom"
	cache2 "github.com/ArtisanCloud/PowerX/boostrap/cache"
	"github.com/ArtisanCloud/PowerX/config"
	app2 "github.com/ArtisanCloud/PowerX/config/app"
	"github.com/ArtisanCloud/PowerX/config/cache"
	database2 "github.com/ArtisanCloud/PowerX/config/database"
	"github.com/ArtisanCloud/PowerX/database"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/ArtisanCloud/PowerX/resources/lang"
	"github.com/gin-gonic/gin"
	"os"
)

func InitProject() (err error) {
	// Initialize the global config
	envConfigName := "environment"
	dbConfigName := "database"
	cacheConfigName := "cache"

	err = app2.LoadEnvConfig(nil, &envConfigName, nil)
	if err != nil {
		return err
	}

	// Initialize the logger
	err = logger.SetupLog()
	if err != nil {
		return err
	}

	err = database2.LoadDatabaseConfig(nil, &dbConfigName, nil)
	if err != nil {
		return err
	}

	err = cache.LoadCacheConfig(nil, &cacheConfigName, nil)
	if err != nil {
		return err
	}

	config.LoadVersion()

	// load locale
	lang.LoadLanguages()

	// setup ssh key path
	service.SetupSSHKeyPath(app2.G_AppConfigure.SSH)

	// Initialize the cache
	err = cache2.SetupCache()
	if err != nil {
		return err
	}

	// Initialize the database
	err = database.SetupDatabase()
	if err != nil {
		return err
	}

	err = InitServices()
	if err != nil {
		return err
	}

	return err
}

func InitServices() (err error) {

	// defined singleton located in app/service/wx/wecom/datetime.go
	if wecom.G_WeComApp == nil {
		wecom.G_WeComApp, err = wecom.NewWeComService(nil)
		if err != nil {
			return err
		}
	}

	// defined singleton located in app/service/wx/wecom/datetime.go
	if wecom.G_WeComEmployee == nil {
		ctx := &gin.Context{}
		ctx.Set("messageToken", os.Getenv("employee_message_token"))
		ctx.Set("messageAESKey", os.Getenv("employee_message_aes_key"))
		ctx.Set("messageCallbackURL", os.Getenv("employee_message_callback_url"))
		wecom.G_WeComEmployee, err = wecom.NewWeComService(ctx)
		if err != nil {
			return err
		}
	}

	// defined singleton located in app/service/wx/wecom/datetime.go
	if wecom.G_WeComCustomer == nil {
		ctx := &gin.Context{}
		ctx.Set("messageToken", os.Getenv("customer_message_token"))
		ctx.Set("messageAESKey", os.Getenv("customer_message_aes_key"))
		ctx.Set("messageCallbackURL", os.Getenv("customer_message_callback_url"))
		wecom.G_WeComCustomer, err = wecom.NewWeComService(ctx)
		if err != nil {
			return err
		}
	}

	// defined singleton located in app/service/wx/miniprogram/datetime.go
	if miniProgram.MiniProgramApp == nil {
		miniProgram.MiniProgramApp, err = miniProgram.NewMiniProgramService(nil)
		if err != nil {
			return err
		}
	}

	//if global.PaymentApp == nil {
	//	global.PaymentApp, err = wecom.NewPaymentService(nil)
	//	if err != nil {
	//		return err
	//	}
	//}

	return err

}
