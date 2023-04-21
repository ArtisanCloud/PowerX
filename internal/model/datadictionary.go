package model

import (
	"PowerX/internal/model/powermodel"
)

// Table Name
func (mdl *PivotDataDictionaryToObject) TableName() string {
	return TableNamePivotDataDictionaryToObject
}

// 数据表结构
// Pivot表
type PivotDataDictionaryToObject struct {
	powermodel.PowerPivot

	ObjectName            string `gorm:"column:object_name; not null;index:idx_object_name;comment:对象表名称" json:"objectName"`
	ObjectID              int64  `gorm:"column:object_id; not null;index:idx_object_id;comment:对象Id" json:"objectID"`
	DataDictionaryType    string `gorm:"column:dd_type; not null;index:idx_dd_type;comment:数据字典类型type" json:"DataDictionaryType"`
	DataDictionaryItemKey string `gorm:"column:dd_item_key; not null;index:idx_dd_item_key;comment:数据字典数据项key" json:"dataDictionaryItemKey"`
}

const TableNamePivotDataDictionaryToObject = "pivot_data_dictionary_to_object"

// 数据字典数据项
type DataDictionaryItem struct {
	powermodel.PowerModel

	DataDictionaryType *DataDictionaryType `gorm:"foreignKey:Type;references:Type" json:"dataDictionaryType"`

	Key         string `gorm:"index:idx_key_type;comment:数据唯一标识key"`
	Type        string `gorm:"index:idx_key_type;comment:数据类型标识"`
	Name        string `gorm:"comment:数据显示名字"`
	Value       string `gorm:"comment:数据计算值"`
	Sort        int    `gorm:"default:0;comment:排序"`
	Description string `gorm:"comment:数据描述"`
}

const DataDictionaryItemUniqueId = powermodel.UniqueId

// 数据字典类型，聚合数据字典
type DataDictionaryType struct {
	powermodel.PowerModel

	Items []*DataDictionaryItem `gorm:"foreignKey:Type;references:Type" json:"items"`

	Type        string `gorm:"unique;comment:数据聚合类型标识key"`
	Name        string `gorm:"comment:数据类型显示名字"`
	Description string `gorm:"comment:数据类型描述"`
}

const DataDictionaryTypeUniqueId = powermodel.UniqueId
