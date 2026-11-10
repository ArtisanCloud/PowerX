package ai

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
	"time"
)

const TableMediaTasks = "ai_media_tasks"

// MediaTask 是媒体业务状态，投递、重试与消费者租约统一使用 TaskBus。
type MediaTask struct {
	ProviderStatus  int
	ProviderTraceID string `gorm:"size:128"`
	model.PowerUUIDModel
	TenantUUID        string         `gorm:"type:uuid;not null;uniqueIndex:uk_ai_media_request,priority:1;index"`
	Subject           string         `gorm:"size:512;not null;uniqueIndex:uk_ai_media_request,priority:2"`
	Capability        string         `gorm:"size:128;not null;uniqueIndex:uk_ai_media_request,priority:3"`
	IdempotencyKey    string         `gorm:"size:128;not null;uniqueIndex:uk_ai_media_request,priority:4"`
	RequestDigest     string         `gorm:"size:64;not null"`
	Env               string         `gorm:"size:32;not null"`
	ModelKey          string         `gorm:"size:256;not null"`
	Status            string         `gorm:"size:16;not null;index"`
	Phase             string         `gorm:"size:32;not null"`
	Revision          int64          `gorm:"not null"`
	TraceID           string         `gorm:"size:128;not null"`
	Request           datatypes.JSON `gorm:"type:jsonb;not null"`
	ProviderTaskID    string         `gorm:"size:256" json:"-"`
	ProviderOutput    datatypes.JSON `gorm:"type:jsonb" json:"-"`
	Result            datatypes.JSON `gorm:"type:jsonb"`
	ErrorCode         string         `gorm:"size:128"`
	ProviderErrorCode string         `gorm:"size:128"`
	CancelStatus      string         `gorm:"size:32;not null"`
	CompletedAt       *time.Time
}

func (*MediaTask) TableName() string { return model.PowerXSchema + "." + TableMediaTasks }
func (m *MediaTask) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return TableMediaTasks
}
