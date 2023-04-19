package model

import (
	"PowerX/internal/model/powermodel"
	"github.com/ArtisanCloud/PowerLibs/v3/database"
)

// Table Name
func (mdl *PivotDataDictionaryToObject) TableName() string {
	return TableNamePivotDataDictionaryToObject
}

// 数据表结构
// Pivot表
type PivotDataDictionaryToObject struct {
	powermodel.PowerPivot

	ObjectName           int64 `gorm:"column:object_name; not null;index:index_object_name" json:"objectName"`
	ObjectID             int64 `gorm:"column:object_id; not null;index:index_object_id" json:"objectID"`
	DataDictionaryItemID int64 `gorm:"column:data_dictionary_item_id; not null;index:index_data_dictionary_item_id" json:"dataDictionaryItemID"`
}

const TableNamePivotDataDictionaryToObject = "pivot_data_dictionary_to_object"

// 数据字典项
type DataDictionaryItem struct {
	DataDictionaryType *DataDictionaryType `gorm:"foreignKey:Type;references:Type" json:"dataDictionaryType"`

	database.PowerModel
	Key         string `gorm:"index:idx_key_type"`
	Type        string `gorm:"index:idx_key_type"`
	Name        string
	Value       string
	Sort        int `gorm:"default:0"`
	Description string
}

const DataDictionaryItemUniqueId = powermodel.UniqueId

// 数据字典类型，聚合数据字典
type DataDictionaryType struct {
	database.PowerModel
	Type        string `gorm:"unique"`
	Name        string
	Description string
}

const DataDictionaryTypeUniqueId = powermodel.UniqueId
