package agent

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	TableServiceSession    = "agent_service_sessions"
	TableServiceMessage    = "agent_service_messages"
	TableServiceInvocation = "agent_service_invocations"
)

// ServiceSession is intentionally separate from historical human chat sessions.
// Ownership cannot be inferred or migrated from a human user to a plugin.
type ServiceSession struct {
	coremodel.PowerUUIDModel
	TenantUUID           uuid.UUID      `gorm:"type:uuid;not null;index:idx_agent_service_owner,priority:1"`
	PluginID             string         `gorm:"size:128;not null;index:idx_agent_service_owner,priority:2"`
	ServiceActor         string         `gorm:"size:256;not null;index:idx_agent_service_owner,priority:3"`
	AgentUUID            uuid.UUID      `gorm:"type:uuid;not null;index"`
	Title                string         `gorm:"size:255;not null"`
	Status               string         `gorm:"size:16;not null"`
	Revision             uint64         `gorm:"not null"`
	PendingTask          datatypes.JSON `gorm:"type:jsonb"`
	PendingTaskExpiresAt *time.Time
}

type ServiceMessage struct {
	coremodel.PowerUUIDModel
	TenantUUID           uuid.UUID      `gorm:"type:uuid;not null;index"`
	SessionUUID          uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uk_agent_service_message_key,priority:1;uniqueIndex:uk_agent_service_message_sequence,priority:1"`
	AgentUUID            uuid.UUID      `gorm:"type:uuid;not null"`
	Role                 string         `gorm:"size:16;not null"`
	Content              string         `gorm:"type:text;not null"`
	ResponseEnvelope     datatypes.JSON `gorm:"type:jsonb"`
	Sequence             uint64         `gorm:"not null;uniqueIndex:uk_agent_service_message_sequence,priority:2"`
	IdempotencyKey       string         `gorm:"size:128;not null;uniqueIndex:uk_agent_service_message_key,priority:2"`
	RequestHash          string         `gorm:"size:64;not null"`
	IdempotencyExpiresAt time.Time      `gorm:"not null"`
}

// An invocation is addressable independently from its SSE subscriptions.
// Reconnecting to one must never create a new execution.
type ServiceInvocation struct {
	coremodel.PowerUUIDModel
	TenantUUID           uuid.UUID `gorm:"type:uuid;not null;index"`
	SessionUUID          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uk_agent_service_invocation_key,priority:1"`
	MessageUUID          uuid.UUID `gorm:"type:uuid;not null"`
	AgentUUID            uuid.UUID `gorm:"type:uuid;not null"`
	PluginID             string    `gorm:"size:128;not null"`
	ServiceActor         string    `gorm:"size:256;not null"`
	Status               string    `gorm:"size:16;not null"`
	RunEnv               string    `gorm:"size:32;index:idx_agent_service_archive_backlog,priority:1"`
	AdmissionState       string    `gorm:"size:24;index:idx_agent_service_recovery,priority:1;index:idx_agent_service_archive_backlog,priority:2"`
	IdempotencyKey       string    `gorm:"size:128;not null;uniqueIndex:uk_agent_service_invocation_key,priority:2"`
	RequestHash          string    `gorm:"size:64;not null"`
	IdempotencyExpiresAt time.Time `gorm:"not null"`
	CancelRequestedAt    *time.Time
	HotExpiresAt         *time.Time     `gorm:"index"`
	ArchiveKey           string         `gorm:"type:text"`
	ArchivedAt           *time.Time     `gorm:"index;index:idx_agent_service_archive_backlog,priority:3"`
	FinishedAt           *time.Time     `gorm:"index:idx_agent_service_recovery,priority:2;index:idx_agent_service_archive_backlog,priority:4"`
	DeadlineAt           time.Time      `gorm:"not null"`
	Output               string         `gorm:"type:text"`
	ResponseEnvelope     datatypes.JSON `gorm:"type:jsonb"`
	ReasonCode           string         `gorm:"size:128"`
	TraceUUID            uuid.UUID      `gorm:"type:uuid;not null"`
}

func (*ServiceSession) TableName() string { return coremodel.PowerXSchema + "." + TableServiceSession }
func (*ServiceMessage) TableName() string { return coremodel.PowerXSchema + "." + TableServiceMessage }
func (*ServiceInvocation) TableName() string {
	return coremodel.PowerXSchema + "." + TableServiceInvocation
}
