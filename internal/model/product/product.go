package product

import (
	"PowerX/internal/model/powermodel"
	"time"
)

type Product struct {
	PriceBooks       []*PriceBook      `gorm:"many2many:public.price_book_entries;foreignKey:Id;joinForeignKey:Id;References:Id;JoinReferences:PriceBookId" json:"priceBooks"`
	PriceBookEntries []*PriceBookEntry `gorm:"foreignKey:ProductId;references:Id" json:"priceBookEntries"`
	//Coupons          []*Coupon         `gorm:"many2many:public.r_product_to_coupon;foreignKey:Id;joinForeignKey:ProductId;References:Id;JoinReferences:CouponId" json:"coupons"`
	//SellPlatform *model.DataDictionary `gorm:"comment:'销售平台'"`

	powermodel.PowerModel

	Name               string    `gorm:"comment:产品名称"`
	Type               int8      `gorm:"comment:产品类型，比如商品，还是服务"`
	Plan               int8      `gorm:"comment:产品计划，比如是周期性产品还是一次性产品"`
	AccountingCategory string    `gorm:"comment:财务类别，方便和财务系统对账和审批"`
	CanSellOnline      bool      `gorm:"comment:是否允许线上销售"`
	CanUseForDeduct    bool      `gorm:"comment:产品购买，是否可以使用抵扣方式"`
	ApprovalStatus     int8      `gorm:"comment:产品上架，是否审核通过"`
	IsActivated        bool      `gorm:"comment:是否被激活"`
	Status             int8      `gorm:"comment:产品状态"`
	Description        string    `gorm:"comment:产品描述"`
	CoverURL           string    `gorm:"comment:产品主图"`
	PurchasedQuantity  int8      `gorm:"comment:允许购买数量上限"`
	ValidityPeriodDays int8      `gorm:"comment:售卖时间期限，按天"`
	SaleStartDate      time.Time `gorm:"comment:售卖开始事件"`
	SaleEndDate        time.Time `gorm:"comment:售卖结束事件"`
}

const ProductUniqueId = powermodel.UniqueId

const ProductTypeGoods = 1
const ProductTypeService = 2
