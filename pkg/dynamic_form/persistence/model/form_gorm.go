package modelForm

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"time"

	"gorm.io/datatypes"
)

// FormSchemaRecord 持久化表单 schema（支持版本/回滚）
type FormSchemaRecord struct {
	ID          string `gorm:"primaryKey;type:varchar(128)"`
	Title       string
	Description string
	Fields      datatypes.JSON // 序列化的字段定义（[]model.Field）
	Variables   datatypes.JSON // map[string]string
	Metadata    datatypes.JSON // 额外
	CreatedAt   time.Time
	UpdatedAt   time.Time
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
	ID           string         `gorm:"primaryKey;type:varchar(128)"`
	FormSchemaID string         `gorm:"index;type:varchar(128)"`
	Input        datatypes.JSON // 原始输入
	Cleaned      datatypes.JSON // 验证后输入
	Errors       datatypes.JSON // 字段级错误
	Context      datatypes.JSON // 动态上下文（可选）
	CreatedAt    time.Time
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
