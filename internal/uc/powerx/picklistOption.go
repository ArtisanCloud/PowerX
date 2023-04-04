package powerx

import (
	"github.com/ArtisanCloud/PowerLibs/v3/database"
)

type PicklistOption struct {
	*database.PowerModel

	Area            string `gorm:"column:area" json:"area" primaryKey;autoIncrement:"false" sql:"index"`
	CurrencyISOCode string `gorm:"column:currencyisocode" json:"currencyisocode"`
	FieldApiName    string `gorm:"column:field_api_name" json:"fieldApiName"`
	FieldLabelName  string `gorm:"column:field_label_name" json:"fieldLabelName"`
	Label           string `gorm:"column:label" json:"label"`
	Locale          string `gorm:"column:locale" json:"locale"`
	Name            string `gorm:"column:name" json:"name"`
	ObjectApiName   string `gorm:"column:object_api_name" json:"objectApiName"`
	ObjectLabelName string `gorm:"column:object_label_name" json:"objectLabelName"`
	OwnerID         string `gorm:"column:ownerid" json:"ownerid"`
	Value           string `gorm:"column:value" json:"value"`
}
