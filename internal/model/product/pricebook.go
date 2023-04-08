package product

import (
	"PowerX/internal/model/powermodel"
	"github.com/ArtisanCloud/PowerLibs/v3/database"
)

// PriceBook 数据表结构
type PriceBook struct {
	Products []*Product `gorm:"many2many:price_book_entries;foreignKey:UUID;joinForeignKey:price_book_uuid;References:UUID;JoinReferences:price_book_uuid"`
	//Resellers []*Reseller `gorm:"foreignKey:PriceBookUUID;references:UUID" json:"resellers"`

	database.PowerModel
	IsStandard bool   `gorm:"column:is_standard" json:"isStandard"`
	Name       string `gorm:"column:name" json:"name"`
	Region     int8   `gorm:"column:region" json:"region"`
	Level      int8   `gorm:"column:level" json:"level"`
	StoreUUID  string `gorm:"column:storeUUID" json:"storeUUID"`
}

const TableNamePriceBook = "price_books"
const PriceBookUniqueId = powermodel.UniqueId
