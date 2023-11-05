package scrm

import (
	"PowerX/internal/config"
	"PowerX/internal/uc/powerx/scrm/wechat"
	"fmt"
	"time"

	"github.com/ArtisanCloud/PowerWeChat/v3/src/work"
	"github.com/robfig/cron/v3"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"gorm.io/gorm"
)

type SCRMUseCase struct {
	conf   *config.Config
	db     *gorm.DB
	kv     *redis.Redis
	Cron   *cron.Cron
	Wework *work.Work
	Wechat wechat.IWechatInterface
	//DTalk
}

func NewSCRMUseCase(db *gorm.DB, conf *config.Config, c *cron.Cron, kv *redis.Redis) *SCRMUseCase {
	wework, err := work.NewWork(&work.UserConfig{
		CorpID:  conf.WeWork.CropId,
		AgentID: conf.WeWork.AgentId,
		Secret:  conf.WeWork.Secret,
		OAuth: work.OAuth{
			Callback: conf.WeWork.OAuth.Callback,
			Scopes:   conf.WeWork.OAuth.Scopes,
		},
		Token:     conf.WeWork.Token,
		AESKey:    conf.WeWork.EncodingAESKey,
		HttpDebug: conf.WeWork.Debug,
	})
	if err != nil {
		panic(err)
	}
	return &SCRMUseCase{
		db:     db,
		Cron:   c,
		Wework: wework,
		Wechat: wechat.Repo(db, wework, kv),
	}
}

// Schedule 定时任务
func (uc *SCRMUseCase) Schedule() {

	sl := []wechat.TimerTypeByte{
		// customer message
		wechat.AppGroupCustomerMessageTimerTypeByte,
		// app group organization message
		wechat.AppGroupOrganizationMessageTimerTypeByte,
		// app message
		wechat.AppMessageTimerTypeByte,
	}

	_, _ = uc.Cron.AddFunc(`*/1 * * * *`, func() {
		var err error
		//  timer 1 minute
		unix := time.Now()

		for _, val := range sl {
			err = uc.Wechat.InvokeTimerMessageGrabUniteSend(val, unix.Unix())
			if err != nil {
				logx.Info(fmt.Sprintf(`--- [%s] cron.schedule.call.message.%d.error, %v.`, unix.String(), val, err))
			}
		}

	})

	go uc.Cron.Start()
}

func (uc *SCRMUseCase) AvailabilityCheck() error {
	if uc.conf.WeWork.CropId == "" || uc.conf.WeWork.AgentId == 0 || uc.conf.WeWork.Secret == "" {
		return fmt.Errorf("scrm.wechat.config.error")
	}
	return nil
}
