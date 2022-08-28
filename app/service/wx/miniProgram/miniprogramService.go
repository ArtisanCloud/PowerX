package miniProgram

import (
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/response"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/miniProgram"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
	"log"
)

type MiniProgramService struct {
	App *miniProgram.MiniProgram
}

func NewMiniProgramService(ctx *gin.Context) (*MiniProgramService, error) {

	log.Printf("MiniProgram app id: %s", config.G_AppConfigure.WXMiniProgramConfig.MiniProgramAppID)

	app, err := miniProgram.NewMiniProgram(&miniProgram.UserConfig{
		AppID:  config.G_AppConfigure.WXMiniProgramConfig.MiniProgramAppID,  // 小程序、公众号或者企业微信的appid
		Secret: config.G_AppConfigure.WXMiniProgramConfig.MiniProgramSecret, // 商户号 appID

		ResponseType: response.TYPE_MAP,
		Log: miniProgram.Log{
			Level: "debug",
			File:  "./wechat.log",
		},
		Cache: kernel.NewRedisClient(&kernel.RedisOptions{
			Addr:     config.G_RedisConfig.Host,
			Password: config.G_RedisConfig.Password,
			DB:       config.G_RedisConfig.DB,
		}),
		HttpDebug: true,
		Debug:     false,
		//"sandbox": true,
	})

	if err != nil {
		return nil, err
	}

	return &MiniProgramService{
		App: app,
	}, nil

}
