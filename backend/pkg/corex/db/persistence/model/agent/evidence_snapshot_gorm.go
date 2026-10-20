package agent

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
)

// EvidenceSnapshot 保存不可变证据原件。Status=0 表示已清理，禁止以当前业务对象补旧版本。
type EvidenceSnapshot struct {
	coremodel.PowerUUIDModel
	TenantUUID   uuid.UUID `gorm:"type:uuid;not null;index:idx_agent_evidence_owner,priority:1"`
	SubjectUUID  uuid.UUID `gorm:"type:uuid;not null;index:idx_agent_evidence_owner,priority:2"`
	RevisionUUID uuid.UUID `gorm:"type:uuid;not null"`
	RunID        string    `gorm:"size:512;not null;index"`
	TraceID      string    `gorm:"size:512;not null;index"`
	SHA256       string    `gorm:"size:71;not null"`
	Payload      []byte    `gorm:"type:bytea;not null"` // 保存精确字节；JSONB 会重排内容并破坏摘要。
	Status       int16     `gorm:"not null;default:1"`
}

func (*EvidenceSnapshot) TableName() string {
	return coremodel.PowerXSchema + ".agent_evidence_snapshots"
}
