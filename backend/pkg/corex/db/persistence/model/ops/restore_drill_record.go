package ops

import (
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

type RestoreDrillStatus string

const (
	RestoreDrillStatusRunning RestoreDrillStatus = "running"
	RestoreDrillStatusSuccess RestoreDrillStatus = "success"
	RestoreDrillStatusFailed  RestoreDrillStatus = "failed"
)

// RestoreDrillRecord 记录恢复演练结果。
type RestoreDrillRecord struct {
	coremodel.PowerUUIDModel

	SourceJobID uint64             `gorm:"column:source_job_id;not null;index:idx_ops_restore_source_job" json:"source_job_id"`
	Status      RestoreDrillStatus `gorm:"column:status;type:varchar(32);not null;index:idx_ops_restore_status" json:"status"`
	RTOSec      int64              `gorm:"column:rto_seconds;not null;default:0" json:"rto_seconds"`
	ReportURI   string             `gorm:"column:report_uri;type:varchar(512)" json:"report_uri,omitempty"`
	Operator    string             `gorm:"column:operator;type:varchar(128);not null" json:"operator"`
	TraceID     string             `gorm:"column:trace_id;type:varchar(128);index:idx_ops_restore_trace" json:"trace_id,omitempty"`
}

func (RestoreDrillRecord) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableOpsRestoreDrillRecords
}

func (m *RestoreDrillRecord) Normalize() {
	m.Status = RestoreDrillStatus(strings.TrimSpace(strings.ToLower(string(m.Status))))
	m.ReportURI = strings.TrimSpace(m.ReportURI)
	m.Operator = strings.TrimSpace(m.Operator)
	m.TraceID = strings.TrimSpace(m.TraceID)
}
