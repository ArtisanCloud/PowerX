package knowledge

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// TenantReleasePolicy stores tenant gray-release matrix.
type TenantReleasePolicy struct {
	coremodel.PowerModel

	MatrixVersion string         `gorm:"column:matrix_version;type:varchar(64);not null" json:"matrixVersion"`
	PilotTenants  datatypes.JSON `gorm:"column:pilot_tenants;type:jsonb" json:"pilotTenants"`
	Batches       datatypes.JSON `gorm:"column:batches;type:jsonb" json:"batches"`
	Guardrails    datatypes.JSON `gorm:"column:guardrails;type:jsonb" json:"guardrails"`
	ApprovedBy    string         `gorm:"column:approved_by;type:varchar(128)" json:"approvedBy"`
	CreatedBy     string         `gorm:"column:created_by;type:varchar(128)" json:"createdBy"`
	Status        string         `gorm:"column:status;type:varchar(32);not null;default:'active'" json:"status"`
}

func (TenantReleasePolicy) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeTenantReleasePolicies
}

// TenantReleaseBatch tracks each batch execution state.
type TenantReleaseBatch struct {
	coremodel.PowerUUIDModel

	PolicyID     uint64         `gorm:"column:policy_id;not null;index" json:"policyId"`
	VersionID    string         `gorm:"column:version_id;type:varchar(128);not null" json:"versionId"`
	BatchIndex   int            `gorm:"column:batch_index;not null" json:"batchIndex"`
	Tenants      datatypes.JSON `gorm:"column:tenants;type:jsonb" json:"tenants"`
	State        string         `gorm:"column:state;type:varchar(32);not null;default:'pending'" json:"state"`
	Alerts       datatypes.JSON `gorm:"column:alerts;type:jsonb" json:"alerts"`
	Metrics      datatypes.JSON `gorm:"column:metrics;type:jsonb" json:"metrics"`
	BatchToken   string         `gorm:"column:batch_token;type:varchar(64);not null;uniqueIndex" json:"batchToken"`
	PromotedAt   *time.Time     `gorm:"column:promoted_at" json:"promotedAt"`
	CompletedAt  *time.Time     `gorm:"column:completed_at" json:"completedAt"`
	RolledBackAt *time.Time     `gorm:"column:rolled_back_at" json:"rolledBackAt"`
}

func (TenantReleaseBatch) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeTenantReleaseBatches
}

func (b *TenantReleaseBatch) EnsureToken() {
	if b.BatchToken == "" {
		b.BatchToken = uuid.NewString()
	}
}
