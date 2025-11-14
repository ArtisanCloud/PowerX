package knowledge

import (
	"time"

	"github.com/google/uuid"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// DecayTask captures decay/gap remediation tasks.
type DecayTask struct {
	coremodel.PowerUUIDModel

	SpaceUUID    uuid.UUID `gorm:"column:space_uuid;type:uuid;not null;index" json:"space_uuid"`
	Category     string    `gorm:"column:category;type:varchar(32);not null" json:"category"`
	Severity     string    `gorm:"column:severity;type:varchar(16);not null" json:"severity"`
	Status       string    `gorm:"column:status;type:varchar(32);not null;default:'open';index" json:"status"`
	DetectedAt   time.Time `gorm:"column:detected_at;not null" json:"detected_at"`
	SLADueAt     time.Time `gorm:"column:sla_due_at;not null" json:"sla_due_at"`
	ResolvedAt   *time.Time `gorm:"column:resolved_at" json:"resolved_at"`
	AssignedTo   string    `gorm:"column:assigned_to;type:varchar(128)" json:"assigned_to"`
	Resolution   string    `gorm:"column:resolution;type:text" json:"resolution"`
	FalsePositive bool     `gorm:"column:false_positive;not null;default:false" json:"false_positive"`
}

func (DecayTask) TableName() string {
	return tableName(coremodel.TableKnowledgeDecayTasks)
}
