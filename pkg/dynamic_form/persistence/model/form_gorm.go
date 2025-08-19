package modelForm

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
)

// FormSchemaRecord 持久化表单 schema（支持版本/回滚）
type FormSchemaRecord struct {
	model.PowerModel
	Title       string         `gorm:"column:title;type:varchar(128)" json:"title"`
	Description string         `gorm:"column:description;type:text"   json:"description"`
	Fields      datatypes.JSON `gorm:"column:fields;type:jsonb"       json:"fields"`    // 序列化的字段定义
	Variables   datatypes.JSON `gorm:"column:variables;type:jsonb"    json:"variables"` // map[string]string
	Metadata    datatypes.JSON `gorm:"column:metadata;type:jsonb"     json:"metadata"`  // 额外

}

func (mdl *FormSchemaRecord) TableName() string {
	return model.PowerXSchema + "." + TableFormSchemaRecord
}

func (mdl *FormSchemaRecord) GetTableName(needFull bool) string {
	tableName := TableFormSchemaRecord
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

// FormSubmission 记录每次表单提交
type FormSubmission struct {
	model.PowerModel
	SchemaID uint64         `gorm:"column:schema_id;index;not null" json:"schema_id"`
	TenantID uint64         `gorm:"column:tenant_id;index;not null" json:"tenant_id"`
	UserID   *uint64        `gorm:"column:user_id;index"            json:"user_id,omitempty"`
	Payload  datatypes.JSON `gorm:"column:payload;type:jsonb"       json:"payload"`  // 提交数据
	Metadata datatypes.JSON `gorm:"column:metadata;type:jsonb"      json:"metadata"` // 额外
	Status   int16          `gorm:"column:status;default:1;index"   json:"status"`   // 1=active

}

func (mdl *FormSubmission) TableName() string {
	return model.PowerXSchema + "." + TableFormSubmission
}

func (mdl *FormSubmission) GetTableName(needFull bool) string {
	tableName := TableFormSubmission
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}
