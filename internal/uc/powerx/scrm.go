package powerx

import (
    "PowerX/internal/config"
    "PowerX/internal/uc/powerx/scrm/wechat"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work"
    "gorm.io/gorm"
)

type SCRMUseCase struct {
    db     *gorm.DB
    Wechat wechat.IWechatInterface
    //DTalk
}

func NewSCRMUseCase(db *gorm.DB, conf *config.Config) *SCRMUseCase {
    wework, err := work.NewWork(&work.UserConfig{
        CorpID:  conf.WeWork.CropId,
        AgentID: conf.WeWork.AgentId,
        Secret:  conf.WeWork.Secret,
        OAuth: work.OAuth{
            Callback: "https://scrm.superman.net.cn/api/webhook/wework/message", //
            Scopes:   nil,
        },
        Token:     conf.WeWork.Token,
        AESKey:    conf.WeWork.EncodingAESKey,
        HttpDebug: true,
    })
    if err != nil {
        panic(err)
    }
    return &SCRMUseCase{
        db:     db,
        Wechat: wechat.Repo(db, wework),
    }
}
