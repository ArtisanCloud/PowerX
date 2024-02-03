package operation

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
	MainMembershipId int64     `gorm:"comment:主会籍Id" json:"mainMembershipId"`
	OrderId          int64     `gorm:"comment:订单Id" json:"orderId"`
	OrderItemId      int64     `gorm:"comment:订单项Id" json:"orderItemId"`
	CustomerId       int64     `gorm:"comment:客户Id" json:"accountId"`
	ProductId        int64     `gorm:"comment:产品Id" json:"productId"`
	StartDate        time.Time `gorm:"comment:开始时间" json:"startDate"`
	EndDate          time.Time `gorm:"comment:结束时间" json:"endDate"`
	Status           int       `gorm:"comment:会籍状态" json:"status"`
	Type             int       `gorm:"comment:计划" json:"type"`
	ExtendPeriod     bool      `gorm:"comment:是否延续" json:"extendPeriod"`
	Plan             int       `gorm:"comment:计划" json:"plan"`
}

const MembershipUniqueId = powermodel.UniqueId

const TypeMembershipType = "_membership_type"
const TypeMembershipStatus = "_membership_status"

const (
	MembershipStatusActive    = "_active"    // 活跃状态
	MembershipStatusInactive  = "_inactive"  // 非活跃状态
	MembershipStatusExpired   = "_expired"   // 已过期
	MembershipStatusCancelled = "_cancelled" // 已取消
	// 添加其他状态...
)

const (
	MembershipTypeBase    = "_base"    // 基础会籍
	MembershipTypeNormal  = "_normal"  // 普通会籍
	MembershipTypePremium = "_premium" // 高级会籍
	MembershipTypeVIP     = "_vip"     // VIP会籍
	MembershipTypeCustom  = "_custom"  // 定制会籍
)
