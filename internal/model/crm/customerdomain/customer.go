package customerdomain

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
)

type ExternalId struct {
	OpenIdInMiniProgram           string `gorm:"index;comment:微信小程序OpenId" json:"openIdInMiniProgram"`
	OpenIdInWeChatOfficialAccount string `gorm:"index;comment:微信公众号OpenId" json:"openIdInWeChatOfficialAccount"`
	OpenIdInWeCom                 string `gorm:"index;comment:企业微信OpenId" json:"openIdInWeCom"`
}

const CustomerUniqueId = "mobile"

type Customer struct {
	Inviter *Customer `gorm:"foreignKey:InviterId;references:Id" json:"inviter"`

	powermodel.PowerModel
	Name        string `gorm:"comment:客户名称" json:"name"`
	Mobile      string `gorm:"unique;not null;comment:店长Id" json:"mobile"`
	Uuid        string `gorm:"comment:识别码;index" json:"uuid"`
	Password    string `gorm:"comment:客户密码" json:"password"`
	Email       string `gorm:"comment:邮箱地址" json:"email"`
	InviterId   int64  `gorm:"comment:邀请方" json:"inviterId"`
	MgmId       int    `gorm:"comment:MGM Id" json:"mgmId"`
	InviteCode  string `gorm:"comment:裂变邀请码" json:"inviteCode"`
	Source      int    `gorm:"comment:注册来源" json:"source"`
	Type        int    `gorm:"comment:类型：个人，企业" json:"type"`
	IsActivated bool   `gorm:"comment:激活状态" json:"isActivated"`
	ExternalId
}

const TypeCustomerType = "_customer_type"

const CustomerPersonal = "_personal"
const CustomerCompany = "_company"

func (mdl *Customer) TableName() string {
	return model.PowerXSchema + "." + model.TableNameCustomer
}

func (mdl *Customer) GetTableName(needFull bool) string {
	tableName := model.TableNameCustomer
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}
