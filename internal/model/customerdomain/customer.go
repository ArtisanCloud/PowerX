package customerdomain

import (
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
	Email       string `gorm:"comment:邮箱地址" json:"email"`
	InviterId   int64  `gorm:"comment:邀请方" json:"inviterId"`
	Source      int    `gorm:"comment:注册来源" json:"source"`
	Type        int    `gorm:"comment:类型：个人，企业" json:"type"`
	IsActivated bool   `gorm:"comment:激活状态" json:"isActivated"`
	ExternalId
}

const TypeCustomerType = "_customer_type"

const CustomerPersonal = "_personal"
const CustomerCompany = "_company"
