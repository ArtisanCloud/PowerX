package product

import (
	"PowerX/internal/model/powermodel"
	"gorm.io/datatypes"
	"time"
)

type ProductSpecific struct {
	Inventory int16          `gorm:"column:inventory; comment:库存" json:"inventory"`
	Weight    float32        `gorm:"column:weight; comment:重量" json:"weight"`
	Volume    float32        `gorm:"column:volume; comment:体积" json:"volume"`
	Encode    string         `gorm:"column:encode; comment:编码" json:"encode"`
	BarCode   string         `gorm:"column:bar_code; comment:条形码" json:"barCode"`
	Extra     datatypes.JSON `gorm:"column:extra; comment:额外信息，可以json方式存储" json:"extra"`
}

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
	Description        string    `gorm:"comment:产品描述"`
	CoverURL           string    `gorm:"comment:产品主图"`
	PurchasedQuantity  int8      `gorm:"comment:允许购买数量上限"`
	ValidityPeriodDays int8      `gorm:"comment:售卖时间期限，按天"`
	SaleStartDate      time.Time `gorm:"comment:售卖开始时间"`
	SaleEndDate        time.Time `gorm:"comment:售卖结束时间"`
	ProductSpecific
}

const ProductUniqueId = powermodel.UniqueId

const ProductTypeGoods = 1
const ProductTypeService = 2
