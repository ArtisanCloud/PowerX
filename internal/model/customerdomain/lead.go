package customerdomain

import (
	"PowerX/internal/model/powermodel"
)

type Lead struct {
	//Inviter *Customer

	powermodel.PowerModel
	Name        string `gorm:"comment:客户名称" json:"name"`
	Mobile      string `gorm:"unique;comment:店长Id" json:"mobile"`
	Email       string `gorm:"comment:邮箱地址" json:"email"`
	InviterID   int64  `gorm:"comment:邀请方" json:"inviterID"`
	Source      int    `gorm:"comment:注册来源" json:"source"`
	Type        int    `gorm:"comment:类型：个人，企业" json:"type"`
	IsActivated bool   `gorm:"comment:激活状态" json:"isActivated"`
	ExternalId
}

const LeadUniqueId = "mobile"
