package knowledge

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// AuditTrailEntry 存储 Knowledge Space 领域的审计轨迹。
type AuditTrailEntry struct {
	coremodel.PowerModel

	SpaceUUID     uuid.UUID      `gorm:"column:space_uuid;type:uuid;not null;index" json:"space_uuid"`
	Action        string         `gorm:"column:action;type:varchar(64);not null" json:"action"`
	Actor         string         `gorm:"column:actor;type:varchar(128);not null" json:"actor"`
	PayloadHash   string         `gorm:"column:payload_hash;type:char(64);not null" json:"payload_hash"`
	Metadata      datatypes.JSON `gorm:"column:metadata;type:jsonb;default:'{}'" json:"metadata"`
	OccurredAt    time.Time      `gorm:"column:occurred_at;not null" json:"occurred_at"`
	RollbackToken string         `gorm:"column:rollback_token;type:varchar(128)" json:"rollback_token,omitempty"`
}

func (AuditTrailEntry) TableName() string {
	return tableName(coremodel.TableKnowledgeAuditTrailEntries)
}
