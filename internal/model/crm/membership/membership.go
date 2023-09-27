package membership

import (
	"PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/model/powermodel"
	"time"
)

type Membership struct {
	Customer *customerdomain.Customer `gorm:"foreignKey:CustomerId;references:id"`

	MainMembership *Membership   `gorm:"foreignKey:MainMembershipId;references:id"`
	SubMemberships []*Membership `gorm:"foreignKey:MainMembershipId;references:id"`

	powermodel.PowerModel

	Name             string    `gorm:"comment:会籍名称" json:"name"`
	MainMembershipId string    `gorm:"comment:主会籍Id" json:"mainMembershipId"`
	OrderId          string    `gorm:"comment:订单Id" json:"orderId"`
	OrderItemId      string    `gorm:"comment:订单项Id" json:"orderItemId"`
	CustomerId       string    `gorm:"comment:客户Id" json:"accountId"`
	ProductId        string    `gorm:"comment:产品Id" json:"productId"`
	StartDate        time.Time `gorm:"comment:开始时间" json:"startDate"`
	EndDate          time.Time `gorm:"comment:结束时间" json:"endDate"`
	Status           int       `gorm:"comment:会籍状态" json:"status"`
	ExtendPeriod     bool      `gorm:"comment:是否延续" json:"extendPeriod"`
	Level            int       `gorm:"comment:级别" json:"level"`
	Plan             int       `gorm:"comment:计划" json:"plan"`
}
