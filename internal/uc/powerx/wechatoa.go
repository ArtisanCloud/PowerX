package powerx

import (
	"PowerX/internal/config"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/officialAccount"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// WechatOfficialAccountUseCase Official Account Use Case
type WechatOfficialAccountUseCase struct {
	App *officialAccount.OfficialAccount
	db  *gorm.DB
}

func NewWechatOfficialAccountUseCase(db *gorm.DB, conf *config.Config) *WechatOfficialAccountUseCase {
	// 初始化微信公众号API SDK
	app, err := officialAccount.NewOfficialAccount(&officialAccount.UserConfig{
		AppID:  conf.WechatOA.AppId,
		Secret: conf.WechatMP.Secret,
		OAuth: officialAccount.OAuth{
			Callback: "https://wechat-mp.artisan-cloud.com/callback",
			Scopes:   nil,
		},
		//Token:     "vlhkaO8PW6UYyRgWCgb3UDF",
		AESKey:    "zUfVSOan3B5a0TTTTTxY6OrB28MTS9OIXXXXXXLaq3q2PhTG",
		HttpDebug: true,
	})

	if err != nil {
		panic(errors.Wrap(err, "official account init failed"))
	}

	return &WechatOfficialAccountUseCase{
		App: app,
		db:  db,
	}
}
