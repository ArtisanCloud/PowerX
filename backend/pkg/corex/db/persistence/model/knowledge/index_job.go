package knowledge

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// IndexJob records the accepted asynchronous action. It is deliberately
// separate from the legacy ingestion job so every Host Contract operation has
// a stable operation, tenant and public UUID.
type IndexJob struct {
	coremodel.PowerUUIDModel

	TenantUUID   string     `gorm:"column:tenant_uuid;type:varchar(128);not null;index" json:"tenant_uuid"`
	SpaceUUID    string     `gorm:"column:space_uuid;type:uuid;not null;index" json:"space_uuid"`
	DocumentUUID *string    `gorm:"column:document_uuid;type:uuid;index" json:"document_uuid,omitempty"`
	Operation    string     `gorm:"column:operation;type:varchar(32);not null;index" json:"operation"`
	Status       string     `gorm:"column:status;type:varchar(32);not null;default:'queued';index" json:"status"`
	ErrorCode    string     `gorm:"column:error_code;type:varchar(64)" json:"error_code,omitempty"`
	StartedAt    *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	CompletedAt  *time.Time `gorm:"column:completed_at" json:"completed_at,omitempty"`
}

func (IndexJob) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeIndexJobs
}
