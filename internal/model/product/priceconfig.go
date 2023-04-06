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

const TABLE_NAME_PRICE_CONFIG = "price_configs"
const OBJECT_NAME_PRICE_CONFIG = "Price_Config"

const TYPE_LIST_PRICE = "List_Price" //这条不是真实的recordtype，不要加入 ARRAY_RECORD_TYPE
const TYPE_MEMBER = "Member"
const TYPE_MEMBER_EARLY_BIRD = "Member_Early_Bird"
const TYPE_EARLY_BIRD = "Early_Bird"
const TYPE_NEWNEW = "Newnew"

func NewPriceConfig() *PriceConfig {
	return &PriceConfig{}
}
