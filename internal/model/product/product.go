package product

import (
	"PowerX/internal/model"
	"time"
)

type Product struct {
	PriceBooks       []*PriceBook      `gorm:"many2many:public.price_book_entries;foreignKey:UUID;joinForeignKey:ProductUUID;References:UUID;JoinReferences:PriceBookUUID" json:"priceBooks"`
	PriceBookEntries []*PriceBookEntry `gorm:"foreignKey:ProductUUID;references:UUID" json:"priceBookEntries"`
	//Coupons          []*Coupon         `gorm:"many2many:public.r_product_to_coupon;foreignKey:UUID;joinForeignKey:ProductUUID;References:UUID;JoinReferences:CouponUUID" json:"coupons"`

	*model.Model

	Name               string
	Type               int8
	Plan               int8
	AccountingCategory string
	CanSellOnline      bool
	Channel            int64
	SellingPlatform    int64
	CanUseForDeduct    bool
	ApprovalStatus     int8
	IsActivated        bool
	Status             int8
	CustomerIdentity   string
	Description        string
	DisplayURL         string
	PurchasedQuantity  int8
	ValidityPeriodDays int8
	SaleStartDate      time.Time
	SaleEndDate        time.Time
	AvailableStartDate time.Time
	AvailableEndDate   time.Time
}
