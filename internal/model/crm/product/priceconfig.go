package product

import (
	"PowerX/internal/model"
	"time"
)

type PriceConfig struct {
	Discount         float32   `gorm:"column:discount;  type:decimal(4,2); comment:折扣设置,如打八折设置0.8" json:"discount"`
	Price            float64   `gorm:"column:price; type:decimal(10,2); comment:设定该场景下的价格" json:"price"`
	Days             int8      `gorm:"column:days; comment:活动场景的价格有效天数" json:"days"`
	Type             int8      `gorm:"column:type; comment:类型" json:"type"`
	PriceBookEntryId int64     `gorm:"column:price_book_entry_id; comment:价格手册条目Id" json:"priceBookEntryId"`
	StartDate        time.Time `gorm:"column:start_date; comment:活动场景开始时间" json:"startDate"`
	EndDate          time.Time `gorm:"column:end_date; comment:活动场景结束时间" json:"endDate"`
}

func (mdl *PriceConfig) TableName() string {
	return model.PowerXSchema + "." + model.TableNamePriceConfig
}

func (mdl *PriceConfig) GetTableName(needFull bool) string {
	tableName := model.TableNamePriceConfig
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

const TypeListPrice = "List_Price"
const TypeMember = "Member"
const TypeMemberEarlyBird = "Member_Early_Bird"
const TypeEarlyBird = "Early_Bird"
const TypeNewNew = "NewNew"

func NewPriceConfig() *PriceConfig {
	return &PriceConfig{}
}
