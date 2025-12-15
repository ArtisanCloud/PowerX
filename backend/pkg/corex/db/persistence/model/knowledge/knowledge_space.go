package knowledge

import (
	"strings"
	"time"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// KnowledgeSpace 记录租户维度的知识空间配置与配额。
type KnowledgeSpace struct {
	coremodel.PowerUUIDModel

	TenantUUID              string         `gorm:"column:tenant_uuid;type:varchar(128);not null;index:idx_knowledge_space_tenant_name,unique" json:"tenant_uuid"`
	SpaceName               string         `gorm:"column:space_name;type:varchar(128);not null;index:idx_knowledge_space_tenant_name,unique" json:"space_name"`
	DepartmentCode          string         `gorm:"column:department_code;type:varchar(64);not null" json:"department_code"`
	Status                  string         `gorm:"column:status;type:varchar(32);not null;default:'draft';index" json:"status"`
	QuotaCPU                int            `gorm:"column:quota_cpu;type:int;not null;default:2" json:"quota_cpu"`
	QuotaStorageGB          int            `gorm:"column:quota_storage_gb;type:int;not null;default:50" json:"quota_storage_gb"`
	PolicyTemplateVersionID uint64         `gorm:"column:policy_template_version_id;not null" json:"policy_template_version_id"`
	FeatureFlags            datatypes.JSON `gorm:"column:feature_flags;type:jsonb;default:'[]'" json:"feature_flags"`
	RetireAt                *time.Time     `gorm:"column:retire_at" json:"retire_at,omitempty"`
	RetentionExpiresAt      *time.Time     `gorm:"column:retention_expires_at" json:"retention_expires_at,omitempty"`
	CreatedBy               string         `gorm:"column:created_by;type:varchar(128)" json:"created_by,omitempty"`
	UpdatedBy               string         `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
	LastAuditedAt           *time.Time     `gorm:"column:last_audited_at" json:"last_audited_at,omitempty"`
	AuditToken              string         `gorm:"column:audit_token;type:varchar(128)" json:"audit_token,omitempty"`
}

func (KnowledgeSpace) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableKnowledgeSpaces
}

const (
	KnowledgeSpaceStatusDraft   = "draft"
	KnowledgeSpaceStatusActive  = "active"
	KnowledgeSpaceStatusPending = "pending_iam"
	KnowledgeSpaceStatusRetired = "retired"
)

// Normalize ensures canonical tenant UUID casing.
func (k *KnowledgeSpace) Normalize() {
	k.TenantUUID = strings.ToLower(strings.TrimSpace(k.TenantUUID))
}
