package customerdomain

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
)

type Lead struct {
	powermodel.PowerModel

	Inviter *Customer `gorm:"foreignKey:InviterId;references:Id" json:"inviter"`

	Name        string `gorm:"comment:客户名称" json:"name"`
	Mobile      string `gorm:"unique;not null;comment:手机号码，唯一标识" json:"mobile"`
	Email       string `gorm:"comment:邮箱地址" json:"email"`
	InviterId   int64  `gorm:"comment:邀请方Id" json:"inviterId"`
	Source      int    `gorm:"comment:注册来源" json:"source"`
	Type        int    `gorm:"comment:类型：个人，企业" json:"type"`
	IsActivated bool   `gorm:"comment:激活状态" json:"isActivated"`
	ExternalId
}

const LeadUniqueId = "mobile"

func (mdl *Lead) TableName() string {
	return model.PowerXSchema + "." + model.TableNameLead
}

func (mdl *Lead) GetTableName(needFull bool) string {
	tableName := model.TableNameLead
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}
