package skills

import (
	"strings"
	"time"

	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

const (
	SkillPackageSourceAgentAuthoring = "agent_authoring"
	SkillPackageSourceExternalImport = "external_import"

	SkillDefinitionDraftStatusDraft           = "draft"
	SkillDefinitionDraftStatusReadyForReview  = "ready_for_review"
	SkillDefinitionDraftStatusInstructionOnly = "instruction_only"
	SkillDefinitionDraftStatusRejected        = "rejected"
	SkillDefinitionDraftStatusPublished       = "published"
	SkillDefinitionRevisionStatusDraft        = "draft"
	SkillDefinitionRevisionStatusPublished    = "published"
	SkillDefinitionRevisionStatusSuperseded   = "superseded"
)

// SkillPackageSource records a durable object-storage source artifact. The
// package bytes never live in PostgreSQL; only its URI, checksum and parsed
// metadata are stored here.
type SkillPackageSource struct {
	coremodel.PowerUUIDModel

	TenantUUID           string         `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_skill_package_source_tenant" json:"tenant_uuid"`
	SourceKind           string         `gorm:"column:source_kind;type:varchar(32);not null;index:idx_skill_package_source_kind" json:"source_kind"`
	ArtifactURI          string         `gorm:"column:artifact_uri;type:text;not null" json:"artifact_uri"`
	Checksum             string         `gorm:"column:checksum;type:varchar(256);not null" json:"checksum"`
	ContentType          string         `gorm:"column:content_type;type:varchar(128);not null" json:"content_type"`
	SourceURL            string         `gorm:"column:source_url;type:text" json:"source_url,omitempty"`
	SourceRef            string         `gorm:"column:source_ref;type:varchar(256)" json:"source_ref,omitempty"`
	ParserVersion        string         `gorm:"column:parser_version;type:varchar(64);not null" json:"parser_version"`
	StandardManifestJSON datatypes.JSON `gorm:"column:standard_manifest_json;type:jsonb;not null;default:'{}'" json:"standard_manifest_json"`
	PowerXExtensionJSON  datatypes.JSON `gorm:"column:powerx_extension_json;type:jsonb;not null;default:'{}'" json:"powerx_extension_json"`
	CreatedByMemberUUID  string         `gorm:"column:created_by_member_uuid;type:char(36);not null" json:"created_by_member_uuid"`
}

func (SkillPackageSource) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableSkillsPackageSources
}

func (m *SkillPackageSource) Normalize() {
	m.TenantUUID = strings.TrimSpace(strings.ToLower(m.TenantUUID))
	m.SourceKind = strings.TrimSpace(strings.ToLower(m.SourceKind))
	m.ArtifactURI = strings.TrimSpace(m.ArtifactURI)
	m.Checksum = strings.TrimSpace(strings.ToLower(m.Checksum))
	m.ContentType = strings.TrimSpace(strings.ToLower(m.ContentType))
	m.SourceURL = strings.TrimSpace(m.SourceURL)
	m.SourceRef = strings.TrimSpace(m.SourceRef)
	m.ParserVersion = strings.TrimSpace(m.ParserVersion)
	m.CreatedByMemberUUID = strings.TrimSpace(strings.ToLower(m.CreatedByMemberUUID))
}

// SkillDefinitionDraft is the tenant-owned editable metadata record. The
// current revision is selected explicitly and is not inferred from a local
// source package or file-system path.
type SkillDefinitionDraft struct {
	coremodel.PowerUUIDModel

	TenantUUID          string         `gorm:"column:tenant_uuid;type:char(36);not null;uniqueIndex:uk_skill_definition_draft_tenant_skill;index:idx_skill_definition_draft_tenant_status" json:"tenant_uuid"`
	SkillID             string         `gorm:"column:skill_id;type:varchar(160);not null;uniqueIndex:uk_skill_definition_draft_tenant_skill" json:"skill_id"`
	DisplayNameI18n     datatypes.JSON `gorm:"column:display_name_i18n;type:jsonb;not null;default:'{}'" json:"display_name_i18n"`
	DescriptionI18n     datatypes.JSON `gorm:"column:description_i18n;type:jsonb;not null;default:'{}'" json:"description_i18n"`
	SourceKind          string         `gorm:"column:source_kind;type:varchar(32);not null" json:"source_kind"`
	PackageSourceUUID   string         `gorm:"column:package_source_uuid;type:char(36);index" json:"package_source_uuid,omitempty"`
	Status              string         `gorm:"column:status;type:varchar(32);not null;index:idx_skill_definition_draft_tenant_status" json:"status"`
	CurrentRevisionUUID string         `gorm:"column:current_revision_uuid;type:char(36);index" json:"current_revision_uuid,omitempty"`
	CreatedByMemberUUID string         `gorm:"column:created_by_member_uuid;type:char(36);not null" json:"created_by_member_uuid"`
	UpdatedByMemberUUID string         `gorm:"column:updated_by_member_uuid;type:char(36);not null" json:"updated_by_member_uuid"`
}

func (SkillDefinitionDraft) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableSkillsDefinitionDrafts
}

func (m *SkillDefinitionDraft) Normalize() {
	m.TenantUUID = strings.TrimSpace(strings.ToLower(m.TenantUUID))
	m.SkillID = strings.TrimSpace(strings.ToLower(m.SkillID))
	m.SourceKind = strings.TrimSpace(strings.ToLower(m.SourceKind))
	m.PackageSourceUUID = strings.TrimSpace(strings.ToLower(m.PackageSourceUUID))
	m.Status = strings.TrimSpace(strings.ToLower(m.Status))
	m.CurrentRevisionUUID = strings.TrimSpace(strings.ToLower(m.CurrentRevisionUUID))
	m.CreatedByMemberUUID = strings.TrimSpace(strings.ToLower(m.CreatedByMemberUUID))
	m.UpdatedByMemberUUID = strings.TrimSpace(strings.ToLower(m.UpdatedByMemberUUID))
}

// SkillDefinitionRevision is an immutable normalized definition snapshot. A
// successful publish points the revision to the canonical object-storage
// package generated from DefinitionJSON.
type SkillDefinitionRevision struct {
	coremodel.PowerUUIDModel

	TenantUUID           string         `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_skill_definition_revision_tenant" json:"tenant_uuid"`
	DraftUUID            string         `gorm:"column:draft_uuid;type:char(36);not null;uniqueIndex:uk_skill_definition_revision_draft_no;index" json:"draft_uuid"`
	RevisionNumber       int            `gorm:"column:revision_number;not null;uniqueIndex:uk_skill_definition_revision_draft_no" json:"revision_number"`
	DefinitionJSON       datatypes.JSON `gorm:"column:definition_json;type:jsonb;not null" json:"definition_json"`
	ChangeSummary        string         `gorm:"column:change_summary;type:text" json:"change_summary,omitempty"`
	SourceMessageUUID    string         `gorm:"column:source_message_uuid;type:char(36);index" json:"source_message_uuid,omitempty"`
	AuthoredByMemberUUID string         `gorm:"column:authored_by_member_uuid;type:char(36);not null" json:"authored_by_member_uuid"`
	Status               string         `gorm:"column:status;type:varchar(32);not null;index:idx_skill_definition_revision_status" json:"status"`
	PublishedArtifactURI string         `gorm:"column:published_artifact_uri;type:text" json:"published_artifact_uri,omitempty"`
	PublishedChecksum    string         `gorm:"column:published_checksum;type:varchar(256)" json:"published_checksum,omitempty"`
	PublishedAt          *time.Time     `gorm:"column:published_at" json:"published_at,omitempty"`
}

func (SkillDefinitionRevision) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableSkillsDefinitionRevisions
}

func (m *SkillDefinitionRevision) Normalize() {
	m.TenantUUID = strings.TrimSpace(strings.ToLower(m.TenantUUID))
	m.DraftUUID = strings.TrimSpace(strings.ToLower(m.DraftUUID))
	m.ChangeSummary = strings.TrimSpace(m.ChangeSummary)
	m.SourceMessageUUID = strings.TrimSpace(strings.ToLower(m.SourceMessageUUID))
	m.AuthoredByMemberUUID = strings.TrimSpace(strings.ToLower(m.AuthoredByMemberUUID))
	m.Status = strings.TrimSpace(strings.ToLower(m.Status))
	m.PublishedArtifactURI = strings.TrimSpace(m.PublishedArtifactURI)
	m.PublishedChecksum = strings.TrimSpace(strings.ToLower(m.PublishedChecksum))
}
