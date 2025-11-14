package knowledge

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// DeltaJob tracks delta sync packages, approval and rollout states.
type DeltaJob struct {
	coremodel.PowerUUIDModel

	SpaceUUID      uuid.UUID      `gorm:"column:space_uuid;type:uuid;not null;index" json:"space_uuid"`
	Source         string         `gorm:"column:source;type:varchar(64);not null" json:"source"`
	PackageURI     string         `gorm:"column:package_uri;type:text" json:"package_uri"`
	Status         string         `gorm:"column:status;type:varchar(32);not null;default:'generated'" json:"status"`
	ApprovalState  string         `gorm:"column:approval_state;type:varchar(32);not null;default:'pending'" json:"approval_state"`
	DiffAccuracy   float64        `gorm:"column:diff_accuracy;type:numeric(5,2);default:0" json:"diff_accuracy"`
	PartialRelease bool           `gorm:"column:partial_release;not null;default:false" json:"partial_release"`
	ApprovedBy     string         `gorm:"column:approved_by;type:varchar(128)" json:"approved_by"`
	ApprovedAt     *time.Time     `gorm:"column:approved_at" json:"approved_at"`
	PublishedAt    *time.Time     `gorm:"column:published_at" json:"published_at"`
	RollbackCount  int            `gorm:"column:rollback_count;not null;default:0" json:"rollback_count"`
	Report         datatypes.JSON `gorm:"column:report;type:jsonb" json:"report"`
	Notes          string         `gorm:"column:notes;type:text" json:"notes"`
}

func (DeltaJob) TableName() string {
	return tableName(coremodel.TableKnowledgeDeltaJobs)
}
