package plugin_sandbox

import (
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// SandboxValidationRun tracks sandbox execution metadata.
type SandboxValidationRun struct {
	coremodel.PowerUUIDModel

	TenantUUID     string         `gorm:"column:tenant_uuid;type:varchar(128);index;not null" json:"tenant_uuid"`
	PluginID       string         `gorm:"column:plugin_id;type:varchar(128);index" json:"plugin_id"`
	Status         string         `gorm:"column:status;type:varchar(32);index" json:"status"`
	Dataset        string         `gorm:"column:dataset;type:varchar(128)" json:"dataset"`
	DatasetVersion string         `gorm:"column:dataset_version;type:varchar(64)" json:"dataset_version"`
	SuiteID        string         `gorm:"column:suite_id;type:varchar(128)" json:"suite_id"`
	ReportURI      string         `gorm:"column:report_uri;type:text" json:"report_uri"`
	Summary        datatypes.JSON `gorm:"column:summary;type:jsonb" json:"summary"`
	Warnings       datatypes.JSON `gorm:"column:warnings;type:jsonb" json:"warnings"`
	StartedAt      time.Time      `gorm:"column:started_at" json:"started_at"`
	CompletedAt    *time.Time     `gorm:"column:completed_at" json:"completed_at,omitempty"`
	CorrelationID  uuid.UUID      `gorm:"column:correlation_id;type:uuid" json:"correlation_id"`
}

func (SandboxValidationRun) TableName() string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return coremodel.TablePluginSandboxRuns
	}
	return schema + "." + coremodel.TablePluginSandboxRuns
}
