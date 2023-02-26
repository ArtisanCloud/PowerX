package uc

import (
	"PowerX/internal/config"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work"
	"github.com/pkg/errors"
)

type WeWorkUseCase struct {
	API *work.Work
}

func newWeWorkUseCase(conf *config.Config) *WeWorkUseCase {
	// 初始化企业微信API SDK
	api, err := work.NewWork(&work.UserConfig{
		CorpID:  conf.WeWork.CropId,
		AgentID: conf.WeWork.AgentId,
		Secret:  conf.WeWork.Secret,
		OAuth: work.OAuth{
			Callback: "https://wecom.artisan-cloud.com/callback",
			Scopes:   nil,
		},
		HttpDebug: true,
	})

	if err != nil {
		panic(errors.Wrap(err, "wework init failed"))
	}

	return &WeWorkUseCase{
		API: api,
	}
}
