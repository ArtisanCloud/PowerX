package plugin_debug

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// DiagnosticReport stores aggregated debug artifacts.
type DiagnosticReport struct {
	coremodel.PowerUUIDModel

	TenantUUID    string         `gorm:"column:tenant_uuid;type:char(36);index;not null" json:"tenant_uuid"`
	PluginID      string         `gorm:"column:plugin_id;type:varchar(128);index" json:"plugin_id"`
	Status        string         `gorm:"column:status;type:varchar(32);not null" json:"status"`
	Summary       datatypes.JSON `gorm:"column:summary;type:jsonb" json:"summary"`
	Metadata      datatypes.JSON `gorm:"column:metadata;type:jsonb" json:"metadata"`
	LogBundleURI  string         `gorm:"column:log_bundle_uri;type:text" json:"log_bundle_uri"`
	TraceID       string         `gorm:"column:trace_id;type:varchar(128)" json:"trace_id"`
	Notes         string         `gorm:"column:notes;type:text" json:"notes"`
	TicketRef     string         `gorm:"column:ticket_ref;type:varchar(128)" json:"ticket_ref"`
	TicketURL     string         `gorm:"column:ticket_url;type:text" json:"ticket_url"`
	Masked        bool           `gorm:"column:masked" json:"masked"`
	CompletedAt   *time.Time     `gorm:"column:completed_at" json:"completed_at,omitempty"`
	CorrelationID uuid.UUID      `gorm:"column:correlation_id;type:uuid" json:"correlation_id"`
}

func (DiagnosticReport) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TablePluginDebugReports
	}
	return schema + "." + coremodel.TablePluginDebugReports
}
