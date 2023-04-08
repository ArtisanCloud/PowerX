package powermodel

import (
	"github.com/ArtisanCloud/PowerLibs/v3/object"
	"gorm.io/gorm"
)

const OperationEventCreate = 1
const OperationEventUpdate = 2
const OperationEventDelete = 3

const OperationResultSuccess = 1
const OperationResultFailed = 2
const OperationResultCancel = 3

// TableName overrides the table name used by price_book to `profiles`
func (mdl *PowerOperationLog) TableName() string {
	return mdl.GetTableName(true)
}

// PowerOperationLog 数据表结构
type PowerOperationLog struct {
	*PowerCompactModel

	OperatorName  *string `gorm:"column:operatorName" json:"operatorName"`
	OperatorTable *string `gorm:"column:operatorTable" json:"operatorTable"`
	OperatorID    *int32  `gorm:"column:operatorID;index" json:"operatorID"`
	Module        *int16  `gorm:"column:module" json:"module"`
	Operate       *string `gorm:"column:operate" json:"operate"`
	Event         *int8   `gorm:"column:event" json:"event"`
	ObjectName    *string `gorm:"column:objectName" json:"objectName"`
	ObjectTable   *string `gorm:"column:objectTable" json:"objectTable"`
	ObjectID      *int32  `gorm:"column:objectID;index" json:"objectID"`
	Result        *int8   `gorm:"column:result" json:"result"`
}

const TableNameOperationLog = "power_operation_log"
const OperationLogUniqueId = CompactUniqueId

func NewPowerOperationLog(mapObject *object.Collection) *PowerOperationLog {

	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	return &PowerOperationLog{
		PowerCompactModel: NewPowerCompactModel(),
		OperatorName:      mapObject.GetStringPointer("operatorName", ""),
		OperatorTable:     mapObject.GetStringPointer("operatorTable", ""),
		OperatorID:        mapObject.GetInt32Pointer("operatorID", 0),
		Module:            mapObject.GetInt16Pointer("module", 0),
		Operate:           mapObject.GetStringPointer("operate", ""),
		Event:             mapObject.GetInt8Pointer("event", 0),
		ObjectName:        mapObject.GetStringPointer("objectName", ""),
		ObjectTable:       mapObject.GetStringPointer("objectTable", ""),
		ObjectID:          mapObject.GetInt32Pointer("objectID", 0),
		Result:            mapObject.GetInt8Pointer("result", 0),
	}
}

// 获取当前 Model 的数据库表名称
func (mdl *PowerOperationLog) GetTableName(needFull bool) string {
	tableName := TableNameOperationLog
	if needFull {
		tableName = "public.ac_" + tableName
	}
	return tableName
}

func (mdl *PowerOperationLog) SaveOps(db *gorm.DB,
	operatorName string, operator ModelInterface,
	module int16, operate string, event int8,
	objectName string, object ModelInterface,
	result int8,
) error {

	operatorTable := ""
	var operatorID int32 = 0
	if operator != nil {
		operatorTable = operator.GetTableName(true)
		operatorID = operator.GetID()
	}
	if operatorName == "" {
		operatorName = "system"
	}

	objectTable := object.GetTableName(true)
	objectID := object.GetID()

	ops := &PowerOperationLog{
		PowerCompactModel: NewPowerCompactModel(),
		OperatorName:      &operatorName,
		OperatorTable:     &operatorTable,
		OperatorID:        &operatorID,
		Module:            &module,
		Operate:           &operate,
		Event:             &event,
		ObjectName:        &objectName,
		ObjectTable:       &objectTable,
		ObjectID:          &objectID,
		Result:            &result,
	}

	db = db.Save(ops)

	return db.Error
}
