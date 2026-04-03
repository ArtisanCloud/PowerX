package skills

import (
	"strings"
	"time"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

const (
	SkillSourceBuiltin    = "builtin"
	SkillSourcePlugin     = "plugin"
	SkillSourceThirdParty = "third_party"

	SkillStatusDraft      = "draft"
	SkillStatusPublished  = "published"
	SkillStatusDeprecated = "deprecated"
	SkillStatusDisabled   = "disabled"
)

// SkillRegistryRecord stores normalized skill registration metadata.
type SkillRegistryRecord struct {
	coremodel.PowerUUIDModel

	SkillID            string         `gorm:"column:skill_id;type:varchar(128);not null;uniqueIndex:uk_skill_registry_skill_version" json:"skill_id"`
	Version            string         `gorm:"column:version;type:varchar(64);not null;uniqueIndex:uk_skill_registry_skill_version" json:"version"`
	Source             string         `gorm:"column:source;type:varchar(32);not null;index:idx_skill_registry_source_status" json:"source"`
	Status             string         `gorm:"column:status;type:varchar(32);not null;index:idx_skill_registry_source_status" json:"status"`
	IsLatestPublished  bool           `gorm:"column:is_latest_published;not null;default:false;index:idx_skill_registry_latest_published" json:"is_latest_published"`
	BundleURI          string         `gorm:"column:bundle_uri;type:text;not null" json:"bundle_uri"`
	Checksum           string         `gorm:"column:checksum;type:varchar(256);not null" json:"checksum"`
	Signature          string         `gorm:"column:signature;type:text" json:"signature,omitempty"`
	ManifestJSON       datatypes.JSON `gorm:"column:manifest_json;type:jsonb;not null;default:'{}'" json:"manifest_json,omitempty"`
	SourceURL          string         `gorm:"column:source_url;type:text" json:"source_url,omitempty"`
	SourceRef          string         `gorm:"column:source_ref;type:varchar(128)" json:"source_ref,omitempty"`
	ImportType         string         `gorm:"column:import_type;type:varchar(32);not null;default:'upload'" json:"import_type"`
	UpdatedBy          string         `gorm:"column:updated_by;type:varchar(128)" json:"updated_by,omitempty"`
	PublishedAt        *time.Time     `gorm:"column:published_at" json:"published_at,omitempty"`
	LatestSwitchedAt   *time.Time     `gorm:"column:latest_switched_at" json:"latest_switched_at,omitempty"`
	ApprovalNote       string         `gorm:"column:approval_note;type:text" json:"approval_note,omitempty"`
	IntegrityPolicyRef string         `gorm:"column:integrity_policy_ref;type:varchar(64)" json:"integrity_policy_ref,omitempty"`
}

func (SkillRegistryRecord) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableSkillsRegistryRecords
}

// Normalize trims and canonicalizes key attributes before persistence.
func (r *SkillRegistryRecord) Normalize() {
	r.SkillID = strings.TrimSpace(strings.ToLower(r.SkillID))
	r.Version = strings.TrimSpace(r.Version)
	r.Source = strings.TrimSpace(strings.ToLower(r.Source))
	r.Status = strings.TrimSpace(strings.ToLower(r.Status))
	r.ImportType = strings.TrimSpace(strings.ToLower(r.ImportType))
}
