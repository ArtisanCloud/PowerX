package model

import (
	"PowerX/internal/model/powermodel"
	"PowerX/pkg/securityx"
	"fmt"
	"gorm.io/gorm"
)

// System
const TypeObjectStatus = "_object_status"

const StatusDraft = "_draft"
const StatusActive = "_active"
const StatusCanceled = "_canceled"
const StatusPending = "_pending"
const StatusInactive = "_inactive"

const TypeApprovalStatus = "_approval_status"

const ApprovalStatusApply = "_apply"
const ApprovalStatusReject = "_reject"
const ApprovalStatusSuccess = "_success"

// Business
const TypePromoteChannel = "_promote_channel"
const TypeSalesChannel = "_sales_channel"
const TypeSourceChannel = "_source_channel"

const ChannelDirect = "_direct"      // 品牌自营
const ChannelWechat = "_wechat"      // 微信
const ChannelTaoBao = "_tao_bao"     // 淘宝
const ChannelJD = "_jd"              // 京东
const ChannelDianPing = "_dian_ping" // 点评
const ChannelMeiTuan = "_mei_tuan"   // 美团
const ChannelDingDing = "_ding_ding" // 钉钉
const ChannelDouYin = "_dou_yin"     // 抖音
const ChannelAlipay = "_alipay"      // 支付宝

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

// Table Name
func (mdl *PivotDataDictionaryToObject) TableName() string {
	return TableNamePivotDataDictionaryToObject
}

// 数据表结构
// Pivot表
type PivotDataDictionaryToObject struct {
	powermodel.PowerPivot

	DataDictionaryItem *DataDictionaryItem `gorm:"foreignKey:DataDictionaryType,DataDictionaryKey;references:Type,Key" json:"dataDictionaryItem"`

	ObjectType         string `gorm:"column:object_type; not null;index:idx_obj_type;comment:对象表名称" json:"objectOwner"`
	ObjectID           int64  `gorm:"column:object_id; not null;index:idx_obj_id;comment:对象Id" json:"objectID"`
	DataDictionaryType string `gorm:"column:data_dictionary_type; not null;index:idx_dd_type;comment:数据字典数据项type" json:"dataDictionaryType"`
	DataDictionaryKey  string `gorm:"column:data_dictionary_key; not null;index:idx_dd_key;comment:数据字典数据项key" json:"dataDictionaryKey"`
}

const TableNamePivotDataDictionaryToObject = "pivot_data_dictionary_to_object"

const PivotDataDictionaryToObjectOwnerKey = "object_type"
const PivotDataDictionaryToObjectForeignKey = "object_id"

func (mdl *PivotDataDictionaryToObject) GetOwnerKey() string {
	// 因为是morphy类型，所以外键是Owner
	return PivotDataDictionaryToObjectOwnerKey
}
func (mdl *PivotDataDictionaryToObject) GetOwnerValue() string {
	return mdl.ObjectType
}

func (mdl *PivotDataDictionaryToObject) GetForeignKey() string {
	return PivotDataDictionaryToObjectForeignKey
}
func (mdl *PivotDataDictionaryToObject) GetForeignValue() int64 {
	return mdl.ObjectID
}

func (mdl *PivotDataDictionaryToObject) GetPivotComposedUniqueID() string {
	key := fmt.Sprintf("%s-%d-%s-%s",
		mdl.GetOwnerKey(),
		mdl.GetOwnerValue(),
		mdl.DataDictionaryType,
		mdl.DataDictionaryKey,
	)
	hashedId := securityx.HashStringData(key)

	return hashedId
}

//--------------------------------------------------------------------

func (mdl *PivotDataDictionaryToObject) GetMorphPivots(db *gorm.DB, where *map[string]interface{}) ([]*PivotDataDictionaryToObject, error) {
	pivots := []*PivotDataDictionaryToObject{}

	db = powermodel.SelectMorphPivot(db, mdl, where)

	result := db.Find(&pivots)

	return pivots, result.Error

}

// --------------------------------------------------------------------
func (mdl *PivotDataDictionaryToObject) MakeMorphPivotsFromObjectToDDs(obj powermodel.ModelInterface, dds []*DataDictionaryItem) ([]*PivotDataDictionaryToObject, error) {
	pivots := []*PivotDataDictionaryToObject{}
	for _, dd := range dds {
		pivot := &PivotDataDictionaryToObject{
			ObjectType:         obj.GetTableName(true),
			ObjectID:           obj.GetForeignReferValue(),
			DataDictionaryType: dd.Type,
			DataDictionaryKey:  dd.Key,
		}
		//pivot.UniqueID = pivot.GetPivotComposedUniqueID()

		pivots = append(pivots, pivot)
	}
	return pivots, nil
}
