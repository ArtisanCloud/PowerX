package miniProgram

import (
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/response"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/miniProgram"
	"github.com/ArtisanCloud/PowerX/configs/cache"
	"github.com/gin-gonic/gin"
	"log"
	"os"
)

type MiniProgramService struct {
	App *miniProgram.MiniProgram
}

func NewMiniProgramService(ctx *gin.Context) (*MiniProgramService, error) {

	log.Printf("MiniProgram app id: %s", os.Getenv("miniprogram_app_id"))

	app, err := miniProgram.NewMiniProgram(&miniProgram.UserConfig{
		AppID:  os.Getenv("miniprogram_app_id"), // 小程序、公众号或者企业微信的appid
		Secret: os.Getenv("miniprogram_secret"), // 商户号 appID

		ResponseType: response.TYPE_MAP,
		Log: miniProgram.Log{
			Level: "debug",
			File:  "./wechat.log",
		},
		Cache: kernel.NewRedisClient(&kernel.RedisOptions{
			Addr:     cache.G_RedisConfig.Host,
			Password: cache.G_RedisConfig.Password,
			DB:       cache.G_RedisConfig.DB,
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
