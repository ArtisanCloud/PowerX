package runtime_host

import (
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	TableSubjects   = "runtime_host_subjects"
	TableTasks      = "runtime_host_tasks"
	TableOperations = "runtime_host_operations"
)

// Subject 为已验证的 STS 主体建立稳定 UUID；不从业务请求认领归属。
type Subject struct {
	model.PowerUUIDModel
	TenantUUID        uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uk_runtime_host_subject,priority:1"`
	CredentialSubject string    `gorm:"type:varchar(512);not null;uniqueIndex:uk_runtime_host_subject,priority:2"`
}

func (*Subject) TableName() string { return model.PowerXSchema + "." + TableSubjects }
func (m *Subject) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return TableSubjects
}

type Task struct {
	model.PowerUUIDModel
	TenantUUID        uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uk_runtime_host_task_idempotency,priority:1;index:idx_runtime_host_task_owner,priority:1"`
	CallerSubjectUUID uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uk_runtime_host_task_idempotency,priority:2;index:idx_runtime_host_task_owner,priority:2"`
	IdempotencyKey    string         `gorm:"type:varchar(128);not null;uniqueIndex:uk_runtime_host_task_idempotency,priority:3"`
	Type              string         `gorm:"type:varchar(128);not null"`
	Payload           datatypes.JSON `gorm:"type:jsonb;not null"`
	RequestDigest     string         `gorm:"type:char(64);not null"`
	State             string         `gorm:"type:varchar(16);not null"`
	Progress          int            `gorm:"not null"`
	Revision          int64          `gorm:"not null"`
	MessageKey        string         `gorm:"type:varchar(256);not null"`
	Result            datatypes.JSON `gorm:"type:jsonb"`
	CompletedAt       *time.Time
}

func (*Task) TableName() string { return model.PowerXSchema + "." + TableTasks }
func (m *Task) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return TableTasks
}

// Operation 是独立可审计对象，禁止记录缓存值、payload、result 或凭证。
type Operation struct {
	model.PowerUUIDModel
	TenantUUID        uuid.UUID  `gorm:"type:uuid;not null;index:idx_runtime_host_operation_owner,priority:1"`
	CallerSubjectUUID uuid.UUID  `gorm:"type:uuid;not null;index:idx_runtime_host_operation_owner,priority:2"`
	TaskUUID          *uuid.UUID `gorm:"type:uuid;index"`
	Action            string     `gorm:"type:varchar(32);not null"`
	Outcome           string     `gorm:"type:varchar(16);not null"`
	KeyDigest         string     `gorm:"type:varchar(128)"`
	RequestID         string     `gorm:"type:varchar(128)"`
}

func (*Operation) TableName() string { return model.PowerXSchema + "." + TableOperations }
func (m *Operation) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return TableOperations
}
