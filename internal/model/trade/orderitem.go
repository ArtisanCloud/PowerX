package trade

import (
	"PowerX/internal/model/membership"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
)

type OrderItem struct {
	*powermodel.PowerModel

	Order       *Order                 `gorm:"foreignKey:OrderId;references:Id" json:"order"`
	Product     *product.Product       `gorm:"foreignKey:ProductId;references:Id" json:"product"`
	ProductBook *product.PriceBook     `gorm:"foreignKey:PriceBookEntryId;references:Id" json:"priceBook"`
	Membership  *membership.Membership `gorm:"foreignKey:OrderItemId;references:Id" json:"membership"`
	//CouponItem  *CouponItem `gorm:"foreignKey:OrderItemId;references:Id" json:"CouponItem"`

	OrderId          string  `gorm:"column:order_id" json:"orderId"`
	PriceBookEntryId string  `gorm:"column:price_book_entry_id" json:"priceBookEntryId"`
	AccountId        string  `gorm:"column:account_id" json:"accountId"`
	ProductId        string  `gorm:"column:product_id" json:"productId"`
	Quantity         int8    `gorm:"column:quantity" json:"quantity"`
	UnitPrice        float64 `gorm:"type:decimal(10,2); comment:是单品价格" json:"unitPrice"`
	ListPrice        float64 `gorm:"type:decimal(10,2); comment:是商品标价" json:"listPrice"`
	SellingPrice     float64 `gorm:"type:decimal(10,2); comment:是实际交易价格" json:"sellingPrice"`
}
