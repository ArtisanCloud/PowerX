package agent

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"time"
)

const TableAdminRunAdmission = "agent_admin_run_admissions"

// AdminRunAdmission 仅保存管理端受理和终态索引；高频状态由 Redis 管理。
type AdminRunAdmission struct {
	coremodel.PowerUUIDModel
	TenantUUID           uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uk_agent_admin_run_request,priority:1"`
	Env                  string     `gorm:"size:32;not null;uniqueIndex:uk_agent_admin_run_request,priority:2;index:idx_agent_admin_run_recovery,priority:1;index:idx_agent_admin_archive_backlog,priority:1"`
	SessionUUID          uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:uk_agent_admin_run_request,priority:3"`
	MessageUUID          uuid.UUID  `gorm:"type:uuid;not null"`
	AgentUUID            uuid.UUID  `gorm:"type:uuid;not null"`
	SubjectUUID          uuid.UUID  `gorm:"type:uuid;not null"`
	MemberUUID           *uuid.UUID `gorm:"type:uuid"`
	SessionID            uint64     `gorm:"not null"`
	MessageID            uint64     `gorm:"not null"`
	AgentID              uint64     `gorm:"not null"`
	IdempotencyKey       string     `gorm:"size:128;not null;uniqueIndex:uk_agent_admin_run_request,priority:4"`
	InputRef             string     `gorm:"type:text;not null"`
	InputChecksum        string     `gorm:"size:64;not null"`
	TraceUUID            uuid.UUID  `gorm:"type:uuid;not null"`
	SnapshotUUID         uuid.UUID  `gorm:"type:uuid;not null"`
	Status               string     `gorm:"size:24;not null"`
	AdmissionState       string     `gorm:"size:24;not null;index:idx_agent_admin_run_recovery,priority:2;index:idx_agent_admin_archive_backlog,priority:2"`
	DeadlineAt           time.Time  `gorm:"not null"`
	HotExpiresAt         *time.Time `gorm:"index"`
	ArchiveKey           string     `gorm:"type:text"`
	ArchivedAt           *time.Time `gorm:"index;index:idx_agent_admin_archive_backlog,priority:3"`
	FinishedAt           *time.Time `gorm:"index:idx_agent_admin_run_recovery,priority:3;index:idx_agent_admin_archive_backlog,priority:4"`
	AssistantMessageUUID *uuid.UUID `gorm:"type:uuid"`
}

func (*AdminRunAdmission) TableName() string {
	return coremodel.PowerXSchema + "." + TableAdminRunAdmission
}
