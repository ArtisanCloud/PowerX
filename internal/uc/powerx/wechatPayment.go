package powerx

import (
	"PowerX/internal/config"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/payment"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// WechatPaymentUseCase Payment Use Case
type WechatPaymentUseCase struct {
	App *payment.Payment
	db  *gorm.DB
}

func NewWechatPaymentUseCase(db *gorm.DB, conf *config.Config) *WechatPaymentUseCase {
	// 初始化微信公众号API SDK
	app, err := payment.NewPayment(&payment.UserConfig{
		AppID:            conf.WechatPay.AppId,
		MchID:            conf.WechatPay.MchID,
		MchApiV3Key:      conf.WechatPay.MchApiV3Key,
		Key:              conf.WechatPay.Key,
		CertPath:         conf.WechatPay.CertPath,
		KeyPath:          conf.WechatPay.KeyPath,
		RSAPublicKeyPath: conf.WechatPay.RSAPublicKeyPath,
		SerialNo:         conf.WechatPay.SerialNo,
		OAuth: payment.OAuth{
			Callback: "https://wechat-mp.artisan-cloud.com/callback",
			Scopes:   nil,
		},
		HttpDebug: true,
	})

	if err != nil {
		panic(errors.Wrap(err, "wework init failed"))
	}

	return &WechatPaymentUseCase{
		App: app,
		db:  db,
	}
}
