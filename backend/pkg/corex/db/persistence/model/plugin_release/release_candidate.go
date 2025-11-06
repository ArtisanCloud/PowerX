package plugin_release

import (
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// PluginReleaseCandidate captures build metadata, scan results and approval state for each submission.
type PluginReleaseCandidate struct {
	coremodel.PowerUUIDModel

	TenantID         string         `gorm:"column:tenant_id;type:varchar(128);not null;index:idx_plugin_release_candidate_tenant_plugin_version,priority:1" json:"tenant_id"`
	PluginID         string         `gorm:"column:plugin_id;type:varchar(128);not null;index:idx_plugin_release_candidate_tenant_plugin_version,priority:2" json:"plugin_id"`
	Version          string         `gorm:"column:version;type:varchar(64);not null;index:idx_plugin_release_candidate_tenant_plugin_version,priority:3" json:"version"`
	BuildArtifactURI string         `gorm:"column:build_artifact_uri;type:text;not null" json:"build_artifact_uri"`
	CommitHash       string         `gorm:"column:commit_hash;type:varchar(64);not null" json:"commit_hash"`
	ReleaseNotes     string         `gorm:"column:release_notes;type:text" json:"release_notes,omitempty"`
	ScanScore        datatypes.JSON `gorm:"column:scan_score;type:jsonb;default:'{}'" json:"scan_score,omitempty"`
	GateStatus       string         `gorm:"column:gate_status;type:varchar(32);not null;default:'pending';index" json:"gate_status"`
	ApprovalStatus   string         `gorm:"column:approval_status;type:varchar(32);not null;default:'draft';index" json:"approval_status"`
	FailureReason    string         `gorm:"column:failure_reason;type:text" json:"failure_reason,omitempty"`
	Labels           datatypes.JSON `gorm:"column:labels;type:jsonb;default:'{}'" json:"labels,omitempty"`
	RollbackPlanID   *uint64        `gorm:"column:rollback_plan_id" json:"rollback_plan_id,omitempty"`
	SubmittedAt      *time.Time     `gorm:"column:submitted_at" json:"submitted_at,omitempty"`
	GatesCheckedAt   *time.Time     `gorm:"column:gates_checked_at" json:"gates_checked_at,omitempty"`
	ApprovedAt       *time.Time     `gorm:"column:approved_at" json:"approved_at,omitempty"`
	AuditRef         *uuid.UUID     `gorm:"column:audit_ref;type:uuid" json:"audit_ref,omitempty"`
	CreatedBy        string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy        string         `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
}

func (PluginReleaseCandidate) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TablePluginReleaseCandidates
}

// EnsureUniqueConstraint returns the fields that must remain unique across records.
func (PluginReleaseCandidate) EnsureUniqueConstraint() []string {
	return []string{"tenant_id", "plugin_id", "version"}
}

const (
	PluginReleaseGateStatusPending = "pending"
	PluginReleaseGateStatusPassed  = "passed"
	PluginReleaseGateStatusFailed  = "failed"

	PluginReleaseApprovalDraft     = "draft"
	PluginReleaseApprovalSubmitted = "submitted"
	PluginReleaseApprovalApproved  = "approved"
	PluginReleaseApprovalRejected  = "rejected"
)
