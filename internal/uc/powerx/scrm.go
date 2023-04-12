package powerx

import (
	"PowerX/internal/config"
	"PowerX/internal/uc/powerx/scrm"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work"
	"gorm.io/gorm"
)

type SCRMUseCase struct {
	db  *gorm.DB
	Org *scrm.OrganizationUseCase
}

func NewSCRMUseCase(db *gorm.DB, conf *config.Config) *SCRMUseCase {
	wework, err := work.NewWork(&work.UserConfig{
		CorpID:  conf.WeWork.CropId,
		AgentID: conf.WeWork.AgentId,
		Secret:  conf.WeWork.Secret,
		OAuth: work.OAuth{
			Callback: "https://wecom.artisan-cloud.com/callback", //
			Scopes:   nil,
		},
		HttpDebug: true,
	})
	if err != nil {
		panic(err)
	}
	return &SCRMUseCase{
		db:  db,
		Org: scrm.NewOrganizationUseCase(db, wework),
	}
}
