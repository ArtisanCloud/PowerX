package devhotload

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// DevHotloadSessionEvent stores lifecycle/log events for a session.
type DevHotloadSessionEvent struct {
	coremodel.PowerModel

	SessionID  uuid.UUID      `gorm:"column:session_id;type:uuid;not null;index:idx_dev_hotload_event_session,priority:1" json:"session_id"`
	EventType  string         `gorm:"column:event_type;type:varchar(64);not null;index:idx_dev_hotload_event_session,priority:2" json:"event_type"`
	Payload    datatypes.JSON `gorm:"column:payload;type:jsonb;default:'{}'" json:"payload,omitempty"`
	Sequence   int64          `gorm:"column:sequence;not null" json:"sequence"`
	OccurredAt time.Time      `gorm:"column:occurred_at;not null;index" json:"occurred_at"`
}

func (DevHotloadSessionEvent) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TableDevHotloadSessionEvents
	}
	return schema + "." + coremodel.TableDevHotloadSessionEvents
}
