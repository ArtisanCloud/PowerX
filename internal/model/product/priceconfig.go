package product

import (
	"time"
)

type PriceConfig struct {
	Discount         float64   `gorm:"column:discount" json:"discount"`
	Price            float64   `gorm:"column:price" json:"price"`
	Days             int8      `gorm:"column:days" json:"days"`
	Type             int8      `gorm:"column:type" json:"type"`
	ProductId        string    `gorm:"column:product_id" json:"productId"`
	StartDate        time.Time `gorm:"column:start_date" json:"startDate"`
	EndDate          time.Time `gorm:"column:end_date" json:"endDate"`
	PriceBookEntryId string    `gorm:"column:price_book_entry_id" json:"priceBookEntryId"`
}

const TableNamePriceConfig = "price_configs"

const TypeListPrice = "List_Price"
const TypeMember = "Member"
const TypeMemberEarlyBird = "Member_Early_Bird"
const TypeEarlyBird = "Early_Bird"
const TypeNewNew = "NewNew"

func NewPriceConfig() *PriceConfig {
	return &PriceConfig{}
}
