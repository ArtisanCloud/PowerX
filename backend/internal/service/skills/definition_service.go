package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
)

const (
	SkillDefinitionSchemaV2 = "powerx.skill-definition/v2"
	SkillPackageParserV1    = "powerx.skill-package-parser/v1"
)

var skillDefinitionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{2,159}$`)

// DefinitionService is the only write path for tenant-created skill
// definitions. It accepts already structured Agent output or a parsed import;
// it never extracts an execution definition from free-form text at runtime.
type DefinitionService struct {
	repo *skillrepo.SkillDefinitionRepository
}

func NewDefinitionService(repo *skillrepo.SkillDefinitionRepository) *DefinitionService {
	if repo == nil {
		panic("skill definition service requires repository")
	}
	return &DefinitionService{repo: repo}
}

type CreateDefinitionDraftInput struct {
	TenantUUID         string
	SkillID            string
	DisplayNameI18n    map[string]string
	DescriptionI18n    map[string]string
	SourceKind         string
	PackageSourceUUID  string
	Definition         map[string]any
	ChangeSummary      string
	SourceMessageUUID  string
	AuthorMemberUUID   string
	InitialDraftStatus string
}

type CreatePackageSourceInput struct {
	TenantUUID          string
	SourceKind          string
	ArtifactURI         string
	Checksum            string
	ContentType         string
	SourceURL           string
	SourceRef           string
	StandardManifest    map[string]any
	PowerXExtension     map[string]any
	CreatedByMemberUUID string
}

type PublishDefinitionInput struct {
	TenantUUID          string
	DraftUUID           string
	ArtifactURI         string
	Checksum            string
	UpdatedByMemberUUID string
}

// CreatePackageSource records an import package after the importer has copied
// it to object storage and parsed its content in an isolated temporary
// workspace. The runtime never reads ArtifactURI directly.
func (s *DefinitionService) CreatePackageSource(ctx context.Context, in CreatePackageSourceInput) (*skillmodel.SkillPackageSource, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("skill.definition_service_unavailable")
	}
	if err := requireUUID(in.TenantUUID, "tenant_uuid"); err != nil {
		return nil, err
	}
	if err := requireUUID(in.CreatedByMemberUUID, "created_by_member_uuid"); err != nil {
		return nil, err
	}
	sourceKind := strings.TrimSpace(strings.ToLower(in.SourceKind))
	if sourceKind != skillmodel.SkillPackageSourceExternalImport && sourceKind != skillmodel.SkillPackageSourceAgentAuthoring {
		return nil, errors.New("skill.package_source_kind_invalid")
	}
	if err := requireObjectURI(in.ArtifactURI); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(in.Checksum)), "sha256:") {
		return nil, errors.New("skill.package_checksum_invalid")
	}
	if strings.TrimSpace(in.ContentType) == "" {
		return nil, errors.New("skill.package_content_type_required")
	}
	if err := validateStandardSkillManifest(in.StandardManifest); err != nil {
		return nil, err
	}
	standardJSON, err := json.Marshal(in.StandardManifest)
	if err != nil {
		return nil, fmt.Errorf("skill.package_standard_manifest_encode_failed: %w", err)
	}
	extensionJSON, err := json.Marshal(nonNilMap(in.PowerXExtension))
	if err != nil {
		return nil, fmt.Errorf("skill.package_extension_encode_failed: %w", err)
	}
	return s.repo.CreatePackageSource(ctx, &skillmodel.SkillPackageSource{
		TenantUUID:           in.TenantUUID,
		SourceKind:           sourceKind,
		ArtifactURI:          in.ArtifactURI,
		Checksum:             in.Checksum,
		ContentType:          in.ContentType,
		SourceURL:            in.SourceURL,
		SourceRef:            in.SourceRef,
		ParserVersion:        SkillPackageParserV1,
		StandardManifestJSON: datatypes.JSON(standardJSON),
		PowerXExtensionJSON:  datatypes.JSON(extensionJSON),
		CreatedByMemberUUID:  in.CreatedByMemberUUID,
	})
}

// CreateDraft persists a normalized, typed definition. Agent authoring and
// package import both use this method; only SourceKind/PackageSourceUUID differ.
func (s *DefinitionService) CreateDraft(ctx context.Context, in CreateDefinitionDraftInput) (*skillmodel.SkillDefinitionDraft, *skillmodel.SkillDefinitionRevision, error) {
	if s == nil || s.repo == nil {
		return nil, nil, errors.New("skill.definition_service_unavailable")
	}
	if err := validateCreateDraftInput(in); err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(in.PackageSourceUUID) != "" {
		source, err := s.repo.GetPackageSource(ctx, in.TenantUUID, in.PackageSourceUUID)
		if err != nil {
			return nil, nil, err
		}
		if source.SourceKind != strings.TrimSpace(strings.ToLower(in.SourceKind)) {
			return nil, nil, errors.New("skill.definition_package_source_kind_mismatch")
		}
	}
	definitionJSON, err := json.Marshal(in.Definition)
	if err != nil {
		return nil, nil, fmt.Errorf("skill.definition_encode_failed: %w", err)
	}
	nameJSON, err := json.Marshal(in.DisplayNameI18n)
	if err != nil {
		return nil, nil, fmt.Errorf("skill.definition_display_name_encode_failed: %w", err)
	}
	descriptionJSON, err := json.Marshal(in.DescriptionI18n)
	if err != nil {
		return nil, nil, fmt.Errorf("skill.definition_description_encode_failed: %w", err)
	}

	draft := &skillmodel.SkillDefinitionDraft{
		TenantUUID:          in.TenantUUID,
		SkillID:             in.SkillID,
		DisplayNameI18n:     datatypes.JSON(nameJSON),
		DescriptionI18n:     datatypes.JSON(descriptionJSON),
		SourceKind:          in.SourceKind,
		PackageSourceUUID:   in.PackageSourceUUID,
		Status:              normalizeDraftStatus(in.InitialDraftStatus),
		CreatedByMemberUUID: in.AuthorMemberUUID,
		UpdatedByMemberUUID: in.AuthorMemberUUID,
	}
	revision := &skillmodel.SkillDefinitionRevision{
		DefinitionJSON:       datatypes.JSON(definitionJSON),
		ChangeSummary:        in.ChangeSummary,
		SourceMessageUUID:    in.SourceMessageUUID,
		AuthoredByMemberUUID: in.AuthorMemberUUID,
		Status:               skillmodel.SkillDefinitionRevisionStatusDraft,
	}
	return s.repo.CreateDraftWithInitialRevision(ctx, draft, revision)
}

func (s *DefinitionService) AppendRevision(ctx context.Context, tenantUUID, draftUUID, updatedByMemberUUID string, definition map[string]any, summary, sourceMessageUUID string) (*skillmodel.SkillDefinitionDraft, *skillmodel.SkillDefinitionRevision, error) {
	if s == nil || s.repo == nil {
		return nil, nil, errors.New("skill.definition_service_unavailable")
	}
	if err := requireUUID(tenantUUID, "tenant_uuid"); err != nil {
		return nil, nil, err
	}
	if err := requireUUID(draftUUID, "draft_uuid"); err != nil {
		return nil, nil, err
	}
	if err := requireUUID(updatedByMemberUUID, "updated_by_member_uuid"); err != nil {
		return nil, nil, err
	}
	if err := optionalUUID(sourceMessageUUID, "source_message_uuid"); err != nil {
		return nil, nil, err
	}
	if err := validatePowerXDefinition(definition); err != nil {
		return nil, nil, err
	}
	raw, err := json.Marshal(definition)
	if err != nil {
		return nil, nil, fmt.Errorf("skill.definition_encode_failed: %w", err)
	}
	return s.repo.AppendRevision(ctx, tenantUUID, draftUUID, updatedByMemberUUID, &skillmodel.SkillDefinitionRevision{
		DefinitionJSON:    datatypes.JSON(raw),
		ChangeSummary:     strings.TrimSpace(summary),
		SourceMessageUUID: strings.TrimSpace(sourceMessageUUID),
	})
}

// PublishCurrentRevision requires the caller to provide the canonical package
// that was generated from the revision. This prevents a publish from silently
// pointing to an untracked local directory or mutable repository checkout.
func (s *DefinitionService) PublishCurrentRevision(ctx context.Context, in PublishDefinitionInput) (*skillmodel.SkillDefinitionDraft, *skillmodel.SkillDefinitionRevision, error) {
	if s == nil || s.repo == nil {
		return nil, nil, errors.New("skill.definition_service_unavailable")
	}
	if err := requireUUID(in.TenantUUID, "tenant_uuid"); err != nil {
		return nil, nil, err
	}
	if err := requireUUID(in.DraftUUID, "draft_uuid"); err != nil {
		return nil, nil, err
	}
	if err := requireUUID(in.UpdatedByMemberUUID, "updated_by_member_uuid"); err != nil {
		return nil, nil, err
	}
	if err := requireObjectURI(in.ArtifactURI); err != nil {
		return nil, nil, err
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(in.Checksum)), "sha256:") {
		return nil, nil, errors.New("skill.package_checksum_invalid")
	}
	revision, err := s.repo.GetCurrentRevision(ctx, in.TenantUUID, in.DraftUUID)
	if err != nil {
		return nil, nil, err
	}
	var definition map[string]any
	if err := json.Unmarshal(revision.DefinitionJSON, &definition); err != nil {
		return nil, nil, fmt.Errorf("skill.definition_decode_failed: %w", err)
	}
	if err := validatePowerXDefinition(definition); err != nil {
		return nil, nil, err
	}
	return s.repo.PublishCurrentRevision(ctx, in.TenantUUID, in.DraftUUID, in.ArtifactURI, in.Checksum, in.UpdatedByMemberUUID)
}

func validateCreateDraftInput(in CreateDefinitionDraftInput) error {
	if err := requireUUID(in.TenantUUID, "tenant_uuid"); err != nil {
		return err
	}
	if err := requireUUID(in.AuthorMemberUUID, "author_member_uuid"); err != nil {
		return err
	}
	if err := optionalUUID(in.SourceMessageUUID, "source_message_uuid"); err != nil {
		return err
	}
	if !skillDefinitionIDPattern.MatchString(strings.TrimSpace(strings.ToLower(in.SkillID))) {
		return errors.New("skill.definition_skill_id_invalid")
	}
	if err := validateLocalizedMap(in.DisplayNameI18n, "display_name_i18n"); err != nil {
		return err
	}
	if err := validateLocalizedMap(in.DescriptionI18n, "description_i18n"); err != nil {
		return err
	}
	sourceKind := strings.TrimSpace(strings.ToLower(in.SourceKind))
	switch sourceKind {
	case skillmodel.SkillPackageSourceAgentAuthoring, skillmodel.SkillPackageSourceExternalImport:
		if err := requireUUID(in.PackageSourceUUID, "package_source_uuid"); err != nil {
			return err
		}
	default:
		return errors.New("skill.definition_source_kind_invalid")
	}
	status := normalizeDraftStatus(in.InitialDraftStatus)
	if status != skillmodel.SkillDefinitionDraftStatusDraft &&
		status != skillmodel.SkillDefinitionDraftStatusReadyForReview &&
		status != skillmodel.SkillDefinitionDraftStatusInstructionOnly {
		return errors.New("skill.definition_initial_status_invalid")
	}
	if sourceKind == skillmodel.SkillPackageSourceExternalImport && status != skillmodel.SkillDefinitionDraftStatusInstructionOnly {
		return errors.New("skill.definition_import_initial_status_invalid")
	}
	if sourceKind == skillmodel.SkillPackageSourceAgentAuthoring && status == skillmodel.SkillDefinitionDraftStatusInstructionOnly {
		return errors.New("skill.definition_agent_authoring_initial_status_invalid")
	}
	return validatePowerXDefinition(in.Definition)
}

func validatePowerXDefinition(definition map[string]any) error {
	if definition == nil {
		return errors.New("skill.definition_required")
	}
	if strings.TrimSpace(toString(definition["schema"])) != SkillDefinitionSchemaV2 {
		return errors.New("skill.definition_schema_invalid")
	}
	executor, ok := definition["executor"].(map[string]any)
	if !ok {
		return errors.New("skill.definition_executor_required")
	}
	typ := strings.TrimSpace(strings.ToLower(toString(executor["type"])))
	switch typ {
	case "llm_prompt":
		if err := validateLocalizedAnyMap(executor["prompt_template_i18n"]); err != nil {
			return errors.New("skill.definition_llm_prompt_i18n_required")
		}
	case "capability":
		if strings.TrimSpace(toString(executor["capability_id"])) == "" {
			return errors.New("skill.definition_capability_id_required")
		}
	case "workflow":
		if strings.TrimSpace(toString(executor["workflow_uuid"])) == "" {
			return errors.New("skill.definition_workflow_uuid_required")
		}
		if err := requireUUID(toString(executor["workflow_uuid"]), "workflow_uuid"); err != nil {
			return err
		}
	case "instruction_only":
		return nil
	default:
		return errors.New("skill.definition_executor_type_invalid")
	}
	return nil
}

func validateLocalizedAnyMap(raw any) error {
	values, ok := raw.(map[string]any)
	if !ok || len(values) == 0 {
		return errors.New("localized_map_required")
	}
	for locale, value := range values {
		if strings.TrimSpace(locale) == "" || strings.TrimSpace(toString(value)) == "" {
			return errors.New("localized_map_invalid")
		}
	}
	return nil
}

func validateStandardSkillManifest(manifest map[string]any) error {
	if manifest == nil {
		return errors.New("skill.package_standard_manifest_required")
	}
	if strings.TrimSpace(toString(manifest["name"])) == "" || strings.TrimSpace(toString(manifest["description"])) == "" {
		return errors.New("skill.package_standard_manifest_invalid")
	}
	return nil
}

func validateLocalizedMap(values map[string]string, field string) error {
	if len(values) == 0 {
		return errors.New("skill.definition_" + field + "_required")
	}
	for locale, value := range values {
		if strings.TrimSpace(locale) == "" || strings.TrimSpace(value) == "" {
			return errors.New("skill.definition_" + field + "_invalid")
		}
	}
	return nil
}

func requireUUID(value, _ string) error {
	if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
		return errors.New("skill.definition_uuid_invalid")
	}
	return nil
}

func optionalUUID(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return requireUUID(value, field)
}

func requireObjectURI(value string) error {
	uri := strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(uri, "local://") && !strings.HasPrefix(uri, "s3://") && !strings.HasPrefix(uri, "minio://") {
		return errors.New("skill.package_object_uri_required")
	}
	return nil
}

func normalizeDraftStatus(value string) string {
	status := strings.TrimSpace(strings.ToLower(value))
	if status == "" {
		return skillmodel.SkillDefinitionDraftStatusDraft
	}
	return status
}

func nonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}
