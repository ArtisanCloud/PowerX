package product

import (
	"time"
)

type PriceConfig struct {
	Discount           float64   `gorm:"column:discount" json:"discount"`
	Price              float64   `gorm:"column:price" json:"price"`
	Days               int8      `gorm:"column:days" json:"days"`
	Type               int8      `gorm:"column:type" json:"type"`
	ProductUUID        string    `gorm:"column:product_uuid" json:"productUUID"`
	StartDate          time.Time `gorm:"column:start_date" json:"startDate"`
	EndDate            time.Time `gorm:"column:end_date" json:"endDate"`
	PriceBookEntryUUID string    `gorm:"column:price_book_entry_uuid" json:"priceBookEntryUUID"`
}

const TableNamePriceConfig = "price_configs"
const ObjectNamePriceConfig = "Price_Config"

const TypeListPrice = "List_Price" //这条不是真实的recordtype，不要加入 ARRAY_RECORD_TYPE
const TypeMember = "Member"
const TypeMemberEarlyBird = "Member_Early_Bird"
const TypeEarlyBird = "Early_Bird"
const TypeNewNew = "NewNew"

func NewPriceConfig() *PriceConfig {
	return &PriceConfig{}
}
