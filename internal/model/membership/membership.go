package membership

import (
	"PowerX/internal/model/customerdomain"
	"github.com/ArtisanCloud/PowerLibs/v3/database"
	"time"
)

type Membership struct {
	Customer *customerdomain.Customer `gorm:"foreignKey:CustomerID;references:id"`

	MainMembership *Membership   `gorm:"foreignKey:MainMembershipID;references:id"`
	SubMemberships []*Membership `gorm:"foreignKey:MainMembershipID;references:id"`

	database.PowerModel

	CustomerID       string    `gorm:"column:account_id" json:"accountID"`
	StartDate        time.Time `gorm:"column:start_date" json:"startDate"`
	EndDate          time.Time `gorm:"column:end_date" json:"endDate"`
	ExtendPeriod     bool      `gorm:"column:extend_period" json:"extendPeriod"`
	MainMembershipID string    `gorm:"column:main_membership_id" json:"mainMembershipID"`
	Name             string    `gorm:"column:name" json:"name"`
	OrderID          string    `gorm:"column:order_id" json:"orderUUID"`
	OrderItemID      string    `gorm:"column:order_item_id" json:"orderItemID"`
	Level            int8      `gorm:"column:level" json:"level"`
	Plan             int8      `gorm:"column:plan" json:"plan"`
	ProductID        string    `gorm:"column:product_id" json:"productID"`
	Status           int8      `gorm:"column:status" json:"status"`
}
