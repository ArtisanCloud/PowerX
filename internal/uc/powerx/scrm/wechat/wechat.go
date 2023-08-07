package wechat

import (
	"PowerX/internal/model/origanzation"
	"PowerX/internal/model/scene"
	"PowerX/internal/model/scrm/app"
	"PowerX/internal/model/scrm/customer"
	"PowerX/internal/model/scrm/organization"
	"PowerX/internal/model/scrm/resource"
	"PowerX/internal/model/scrm/tag"
	"PowerX/internal/types"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/power"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/response"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"gorm.io/gorm"
	"sync"
)

var Scrm IWechatInterface = new(wechatUseCase)

type wechatUseCase struct {
	//
	//  help
	//  @Description:
	//
	help
	//
	//  db
	//  @Description:
	//
	db *gorm.DB
	//
	//  kv
	//  @Description:
	//
	kv *redis.Redis
	//
	//  wework
	//  @Description:
	//
	wework *work.Work
	//
	//  ctx
	//  @Description:
	//
	ctx context.Context
	//
	//  gLock
	//  @Description:
	//
	gLock *sync.WaitGroup
	//
	//  modelOrganization
	//  @Description:
	//
	modelOrganization
	//
	//  modelWeworkApp
	//  @Description:
	//
	modelWeworkApp
	//
	//  modelWeworkOrganization
	//  @Description:
	//
	modelWeworkOrganization
	//
	//  modelWeworkResource
	//  @Description:
	//
	modelWeworkResource
	//
	//  modelWeworkQrcode
	//  @Description:
	//
	modelWeworkQrcode
	//
	//  modelWeworkTag
	//  @Description:
	//
	modelWeworkTag
	//
	//  modelWeworkCustomer
	//  @Description:
	//
	modelWeworkCustomer
}

type (
	help              struct{}
	hash              power.HashMap
	modelOrganization struct {
		employee   origanzation.Employee
		department origanzation.Department
	}
	modelWeworkApp struct {
		group app.WeWorkAppGroup
	}
	modelWeworkOrganization struct {
		employee   organization.WeWorkEmployee
		department organization.WeWorkDepartment
	}
	modelWeworkResource struct {
		resource resource.WeWorkResource
	}
	modelWeworkQrcode struct {
		qrcode scene.SceneQrcode
	}
	modelWeworkTag struct {
		tag   tag.WeWorkTag
		group tag.WeWorkTagGroup
	}
	modelWeworkCustomer struct {
		follow customer.WeWorkExternalContactFollow
	}
)

// NewOrganizationUseCase
//
//	@Description:
//	@param db
//	@param wework
//	@return iEmployeeInterface
func Repo(db *gorm.DB, wework *work.Work, kv *redis.Redis) IWechatInterface {

	return &wechatUseCase{
		db:     db,
		kv:     kv,
		wework: wework,
		ctx:    context.TODO(),
		gLock:  new(sync.WaitGroup),
	}

}

var (
	HRedisScrmGroupMessageKey = `scrm:app:group:%d`
)

type TimerTypeByte int

const (
	//app message
	AppMessageTimerTypeByte TimerTypeByte = iota + 1<<2
	//app group organization message
	AppGroupOrganizationMessageTimerTypeByte
	//app group customer message
	AppGroupCustomerMessageTimerTypeByte
)

// FindManyWechatDepartmentsOption
// @Description:
type FindManyWechatDepartmentsOption struct {
	WeWorkDepId []int
	Name        string
}

// FindManyWechatEmployeesOption
// @Description:
type FindManyWechatEmployeesOption struct {
	WeWorkUserId           string `json:"we_work_user_id"` //员工唯一ID
	Ids                    []int64
	Names                  []string
	Alias                  []string
	Emails                 []string
	Mobile                 []string
	OpenUserId             []string
	WeWorkMainDepartmentId []int64
	Status                 []int
	types.PageEmbedOption
}

type (

	// https://developer.work.weixin.qq.com/document/path/90248
	WechatAppRequestBase struct {
		ChatIds []string             `json:"chatIds"`
		MsgType string               `json:"msgtype"`
		Safe    int                  `json:"safe"`
		News    WechatAppRequestNews `json:"news"`
	}

	WechatAppRequestNews struct {
		Article []*WechatAppRequestNewsArticle `json:"articles"`
	}

	WechatAppRequestNewsArticle struct {
		Title       string `json:"title"`       // "领奖通知",
		Description string `json:"description"` // "<div class=\"gray\">2016年9月26日</div> <div class=\"normal\">恭喜你抽中iPhone 7一台，领奖码：xxxx</div><div class=\"highlight\">请于2016年10月10日前联系行政同事领取</div>",
		URL         string `json:"url"`         // "URL",
		PicURL      string `json:"picurl"`      // 多"
		AppID       string `json:"appid,omitempty"`
		PagePath    string `json:"pagepath,omitempty"`
	}
)

type (

	//
	//  FindManyWechatCustomerOption
	//  @Description:
	//
	FindManyWechatCustomerOption struct {
		UserId string `json:"user_id"`
		Name   string `gorm:"column:name" json:"name"`
		TagId  string `json:"tag_id"`
		types.PageEmbedOption
	}
)

// decode
//
//	@Description:
//	@receiver self
//	@param str
//	@param body
//	@return help
func (self help) decode(str string, body interface{}) help {

	_ = json.Unmarshal([]byte(str), &body)
	return self
}

// error
//
//	@Description:
//	@receiver self
//	@param ps
//	@param rsp
//	@return err
func (self help) error(ps string, rsp response.ResponseWork) (err error) {

	if rsp.ErrCode > 0 {
		marshal, _ := json.Marshal(rsp)
		err = fmt.Errorf(`%s.%s`, ps, string(marshal))
		logx.Error(err)
	}
	return err

}
