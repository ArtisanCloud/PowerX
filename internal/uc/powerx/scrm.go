package powerx

import (
	"PowerX/internal/config"
	"PowerX/internal/uc/powerx/scrm/wechat"
	"fmt"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work"
	"github.com/robfig/cron/v3"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"gorm.io/gorm"
	"time"
)

type SCRMUseCase struct {
	db     *gorm.DB
	kv     *redis.Redis
	Cron   *cron.Cron
	Wechat wechat.IWechatInterface
	//DTalk
}

func NewSCRMUseCase(db *gorm.DB, conf *config.Config, c *cron.Cron, kv *redis.Redis) *SCRMUseCase {
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
		Cron:   c,
		Wechat: wechat.Repo(db, wework, kv),
	}
}

// Schedule
//
//	@Description:
//	@receiver this
func (this *SCRMUseCase) Schedule() {

	sl := []wechat.TimerTypeByte{
		// customer message
		wechat.AppGroupCustomerMessageTimerTypeByte,
		// app group organization message
		wechat.AppGroupOrganizationMessageTimerTypeByte,
		// app message
		wechat.AppMessageTimerTypeByte,
	}

	_, _ = this.Cron.AddFunc(`*/1 * * * *`, func() {
		var err error
		//  timer 1 minute
		unix := time.Now()

		for _, val := range sl {
			err = this.Wechat.InvokeTimerMessageGrabUniteSend(val, unix.Unix())
			if err != nil {
				logx.Info(fmt.Sprintf(`--- [%s] cron.schedule.call.message.%d.error, %v.`, unix.String(), val, err))
			}
		}

	})

	go this.Cron.Start()

}
