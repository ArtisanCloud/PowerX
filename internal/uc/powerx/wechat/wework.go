package wechat

import (
	"PowerX/internal/config"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type WeWorkUseCase struct {
	API *work.Work
	db  *gorm.DB
}

func NewWeWorkUseCase(db *gorm.DB, conf *config.Config) *WeWorkUseCase {
	// 初始化企业微信API SDK
	api, err := work.NewWork(&work.UserConfig{
		CorpID:  conf.WeWork.CropId,
		AgentID: conf.WeWork.AgentId,
		Secret:  conf.WeWork.Secret,
		OAuth: work.OAuth{
			Callback: "https://wecom.artisan-cloud.com/callback",
			Scopes:   nil,
		},
		Token:     conf.WeWork.Token,
		AESKey:    conf.WeWork.EncodingAESKey,
		HttpDebug: true,
	})

	if err != nil {
		panic(errors.Wrap(err, "wework init failed"))
	}

	return &WeWorkUseCase{
		API: api,
		db:  db,
	}
}
