package modelForm

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
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

// AutoMigrate 执行迁移
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&FormSchemaRecord{},
		&FormSubmission{},
	)
}
