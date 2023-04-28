package membership

import (
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/powermodel"
	"time"
)

type Membership struct {
	Customer *customerdomain.Customer `gorm:"foreignKey:CustomerId;references:id"`

	MainMembership *Membership   `gorm:"foreignKey:MainMembershipId;references:id"`
	SubMemberships []*Membership `gorm:"foreignKey:MainMembershipId;references:id"`

	powermodel.PowerModel

	CustomerId       string    `gorm:"column:account_id" json:"accountId"`
	StartDate        time.Time `gorm:"column:start_date" json:"startDate"`
	EndDate          time.Time `gorm:"column:end_date" json:"endDate"`
	ExtendPeriod     bool      `gorm:"column:extend_period" json:"extendPeriod"`
	MainMembershipId string    `gorm:"column:main_membership_id" json:"mainMembershipId"`
	Name             string    `gorm:"column:name" json:"name"`
	OrderId          string    `gorm:"column:order_id" json:"orderId"`
	OrderItemId      string    `gorm:"column:order_item_id" json:"orderItemId"`
	Level            int8      `gorm:"column:level" json:"level"`
	Plan             int8      `gorm:"column:plan" json:"plan"`
	ProductId        string    `gorm:"column:product_id" json:"productId"`
	Status           int8      `gorm:"column:status" json:"status"`
}
