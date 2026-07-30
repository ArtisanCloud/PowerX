package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
)

var (
	ErrWorkflowPackSeedUnavailable = errors.New("workflow.pack_seed_unavailable")
	ErrWorkflowPackInvalid         = errors.New("workflow.pack_invalid")
)

var workflowPackSeedActorUUID = uuid.MustParse("00000000-0000-0000-0000-000000000006")

type WorkflowPack struct {
	WorkflowKey          string           `json:"workflow_key" yaml:"workflow_key"`
	DisplayNameI18nKey   string           `json:"display_name_i18n_key" yaml:"display_name_i18n_key"`
	DescriptionI18nKey   string           `json:"description_i18n_key" yaml:"description_i18n_key"`
	Category             string           `json:"category" yaml:"category"`
	Version              int32            `json:"version" yaml:"version"`
	OwnerScope           string           `json:"owner_scope" yaml:"owner_scope"`
	RequiredNodeKinds    []string         `json:"required_node_kinds" yaml:"required_node_kinds"`
	RequiredCapabilities []string         `json:"required_capabilities" yaml:"required_capabilities"`
	Steps                []StepDefinition `json:"steps" yaml:"steps"`
	DefaultRetryPolicy   map[string]any   `json:"default_retry_policy" yaml:"default_retry_policy"`
	CompensationPolicy   map[string]any   `json:"compensation_policy" yaml:"compensation_policy"`
	SlaPolicy            map[string]any   `json:"sla_policy" yaml:"sla_policy"`
	Metadata             map[string]any   `json:"metadata" yaml:"metadata"`
	SourcePath           string           `json:"source_path,omitempty" yaml:"-"`
	Checksum             string           `json:"checksum,omitempty" yaml:"-"`
}

type WorkflowPackSeedInput struct {
	TenantUUID string
	ConfigDir  string
	Keys       []string
}

type WorkflowPackSeedResult struct {
	Seeded  []modelworkflow.WorkflowPackSeedRecord
	Skipped []string
}

func LoadWorkflowPacks(configDir string) ([]WorkflowPack, error) {
	configDir = strings.TrimSpace(configDir)
	if configDir == "" {
		configDir = filepath.Join("backend", "config", "workflow_packs")
	}
	var packs []WorkflowPack
	err := filepath.WalkDir(configDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var pack WorkflowPack
		if err := yaml.Unmarshal(raw, &pack); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrWorkflowPackInvalid, path, err)
		}
		pack.SourcePath = path
		sum := sha256.Sum256(raw)
		pack.Checksum = hex.EncodeToString(sum[:])
		if err := ValidateWorkflowPack(pack); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrWorkflowPackInvalid, path, err)
		}
		packs = append(packs, pack)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(packs, func(i, j int) bool {
		return packs[i].WorkflowKey < packs[j].WorkflowKey
	})
	return packs, nil
}

func ValidateWorkflowPack(pack WorkflowPack) error {
	if strings.TrimSpace(pack.WorkflowKey) == "" {
		return errors.New("workflow_key is required")
	}
	if strings.TrimSpace(pack.Category) == "" {
		return errors.New("category is required")
	}
	if pack.Version <= 0 {
		return errors.New("version is required")
	}
	if len(pack.RequiredNodeKinds) == 0 {
		return errors.New("required_node_kinds is required")
	}
	if len(pack.Steps) == 0 {
		return errors.New("steps is required")
	}
	if _, err := ValidateStepDefinitions(pack.Steps); err != nil {
		return err
	}
	return nil
}

func (s *Service) SeedWorkflowPacks(ctx context.Context, input WorkflowPackSeedInput) (WorkflowPackSeedResult, error) {
	result := WorkflowPackSeedResult{}
	if s == nil || s.packSeeds == nil || s.packInstalls == nil || s.definitions == nil || s.adapters == nil {
		return result, ErrWorkflowPackSeedUnavailable
	}
	tenantUUID, err := normalizeTenantUUID(input.TenantUUID)
	if err != nil {
		return result, err
	}
	packs, err := LoadWorkflowPacks(input.ConfigDir)
	if err != nil {
		return result, err
	}
	keyFilter := normalizeKeyFilter(input.Keys)
	for _, pack := range packs {
		if len(keyFilter) > 0 {
			if _, ok := keyFilter[pack.WorkflowKey]; !ok {
				continue
			}
		}
		if err := s.validateWorkflowPackDependencies(pack); err != nil {
			return result, err
		}
		definitionCurrent := false
		installation, err := s.packInstalls.GetByTenantKey(ctx, tenantUUID, pack.WorkflowKey)
		if err == nil {
			if installation.Status == modelworkflow.WorkflowPackInstallationStatusDeleted || installation.Status == modelworkflow.WorkflowPackInstallationStatusDisabled {
				result.Skipped = append(result.Skipped, pack.WorkflowKey)
				continue
			}
			if installation.Checksum == pack.Checksum {
				definitionCurrent, err = s.workflowPackDefinitionCurrent(ctx, tenantUUID, pack, installation)
				if err != nil {
					return result, err
				}
			}
			if installation.Status == modelworkflow.WorkflowPackInstallationStatusEnabled && installation.Checksum == pack.Checksum && definitionCurrent {
				result.Skipped = append(result.Skipped, pack.WorkflowKey)
				continue
			}
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return result, err
		}
		latest, latestErr := s.packSeeds.GetLatestByKey(ctx, tenantUUID, pack.WorkflowKey)
		if latestErr == nil && installation == nil {
			return result, fmt.Errorf("workflow pack installation missing for existing seed record: tenant_uuid=%s workflow_key=%s definition_uuid=%s", tenantUUID, pack.WorkflowKey, latest.DefinitionUUID)
		}
		if latestErr != nil && !errors.Is(latestErr, gorm.ErrRecordNotFound) {
			return result, latestErr
		}
		if latestErr == nil && latest.Checksum == pack.Checksum && installation != nil && definitionCurrent {
			result.Skipped = append(result.Skipped, pack.WorkflowKey)
			continue
		}
		record, err := s.seedWorkflowPack(ctx, tenantUUID, pack)
		if err != nil {
			return result, err
		}
		lastSeededAt := record.SeededAt
		if _, err := s.packInstalls.UpsertEnabled(ctx, &modelworkflow.WorkflowPackInstallation{
			TenantUUID:        tenantUUID,
			WorkflowKey:       pack.WorkflowKey,
			Version:           pack.Version,
			Checksum:          pack.Checksum,
			DefinitionUUID:    record.DefinitionUUID,
			DefinitionVersion: record.DefinitionVersion,
			Source:            "builtin",
			InstalledAt:       &lastSeededAt,
			LastSeededAt:      &lastSeededAt,
			LastAction:        "install",
		}); err != nil {
			return result, err
		}
		result.Seeded = append(result.Seeded, *record)
	}
	return result, nil
}

func (s *Service) ValidateWorkflowPackCatalog(configDir string, keys []string) ([]WorkflowPack, error) {
	if s == nil || s.adapters == nil {
		return nil, ErrWorkflowPackSeedUnavailable
	}
	packs, err := LoadWorkflowPacks(configDir)
	if err != nil {
		return nil, err
	}
	keyFilter := normalizeKeyFilter(keys)
	validated := make([]WorkflowPack, 0, len(packs))
	for _, pack := range packs {
		if len(keyFilter) > 0 {
			if _, ok := keyFilter[pack.WorkflowKey]; !ok {
				continue
			}
		}
		if err := s.validateWorkflowPackDependencies(pack); err != nil {
			return nil, err
		}
		validated = append(validated, pack)
	}
	return validated, nil
}

func (s *Service) ListWorkflowPacks(ctx context.Context, tenantUUID string, keyword string, limit, offset int) ([]modelworkflow.WorkflowPackSeedRecord, int64, error) {
	if s == nil || s.packSeeds == nil {
		return nil, 0, ErrWorkflowPackSeedUnavailable
	}
	normalizedTenantUUID, err := normalizeTenantUUID(tenantUUID)
	if err != nil {
		return nil, 0, err
	}
	return s.packSeeds.ListByTenant(ctx, normalizedTenantUUID, keyword, limit, offset)
}

func (s *Service) GetWorkflowPack(ctx context.Context, tenantUUID string, workflowKey string) (*modelworkflow.WorkflowPackSeedRecord, error) {
	if s == nil || s.packSeeds == nil {
		return nil, ErrWorkflowPackSeedUnavailable
	}
	normalizedTenantUUID, err := normalizeTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	return s.packSeeds.GetLatestByKey(ctx, normalizedTenantUUID, workflowKey)
}

func (s *Service) validateWorkflowPackDependencies(pack WorkflowPack) error {
	for _, nodeKind := range pack.RequiredNodeKinds {
		if _, err := s.adapters.Adapter(nodeKind); err != nil {
			return fmt.Errorf("%w: %s", err, pack.WorkflowKey)
		}
	}
	for _, step := range pack.Steps {
		adapter, err := s.adapters.Adapter(step.NodeKind)
		if err != nil {
			return fmt.Errorf("%w: %s.%s", err, pack.WorkflowKey, step.ID)
		}
		if err := adapter.Validate(step); err != nil {
			return fmt.Errorf("%w: %s.%s: %v", ErrWorkflowPackInvalid, pack.WorkflowKey, step.ID, err)
		}
	}
	return nil
}

func (s *Service) seedWorkflowPack(ctx context.Context, tenantUUID string, pack WorkflowPack) (*modelworkflow.WorkflowPackSeedRecord, error) {
	version, err := s.definitions.NextVersion(ctx, tenantUUID, pack.WorkflowKey)
	if err != nil {
		return nil, err
	}
	stepJSON, err := json.Marshal(pack.Steps)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	definition := &modelworkflow.WorkflowDefinition{
		TenantUUID:           tenantUUID,
		Name:                 pack.WorkflowKey,
		Description:          pack.DescriptionI18nKey,
		Version:              version,
		Status:               "published",
		StepGraph:            datatypes.JSON(stepJSON),
		DefaultRetryPolicy:   toJSONOrEmpty(pack.DefaultRetryPolicy),
		CompensationPolicy:   toJSONOrEmpty(pack.CompensationPolicy),
		SlaPolicy:            toJSONOrEmpty(pack.SlaPolicy),
		Metadata:             toJSONOrEmpty(workflowPackDefinitionMetadata(pack)),
		CreatedBy:            workflowPackSeedActorUUID,
		PublishedAt:          &now,
		LastPublishedBy:      workflowPackSeedActorUUID,
		VersionAlias:         fmt.Sprintf("pack-v%d", pack.Version),
		InitialContextSchema: datatypes.JSON([]byte(`{}`)),
		InputSchema:          datatypes.JSON([]byte(`{}`)),
		WorkflowPackKey:      pack.WorkflowKey,
		SourceType:           "workflow_pack",
		Checksum:             pack.Checksum,
	}
	created, err := s.definitions.CreateDefinition(ctx, definition)
	if err != nil {
		return nil, err
	}
	return s.packSeeds.CreateRecord(ctx, &modelworkflow.WorkflowPackSeedRecord{
		TenantUUID:        tenantUUID,
		WorkflowKey:       pack.WorkflowKey,
		Version:           pack.Version,
		DefinitionUUID:    created.UUID,
		DefinitionVersion: created.Version,
		Checksum:          pack.Checksum,
		Source:            "builtin",
		SeededAt:          now,
	})
}

func (s *Service) workflowPackDefinitionCurrent(ctx context.Context, tenantUUID string, pack WorkflowPack, installation *modelworkflow.WorkflowPackInstallation) (bool, error) {
	if installation == nil || installation.DefinitionUUID == uuid.Nil || installation.DefinitionVersion <= 0 {
		return false, nil
	}
	version := installation.DefinitionVersion
	definition, err := s.definitions.GetByUUID(ctx, tenantUUID, installation.DefinitionUUID, &version)
	if err != nil {
		return false, err
	}
	metadata := map[string]any{}
	if len(definition.Metadata) > 0 {
		if err := json.Unmarshal(definition.Metadata, &metadata); err != nil {
			return false, err
		}
	}
	expected := workflowPackDefinitionMetadata(pack)
	for _, key := range []string{"category", "workflow_pack_key", "display_name_i18n_key", "description_i18n_key", "owner_scope"} {
		if strings.TrimSpace(fmt.Sprint(metadata[key])) != strings.TrimSpace(fmt.Sprint(expected[key])) {
			return false, nil
		}
	}
	return true, nil
}

func workflowPackDefinitionMetadata(pack WorkflowPack) map[string]any {
	metadata := map[string]any{}
	for key, value := range pack.Metadata {
		metadata[key] = value
	}
	metadata["category"] = strings.TrimSpace(strings.ToLower(pack.Category))
	metadata["workflow_pack_key"] = strings.TrimSpace(pack.WorkflowKey)
	metadata["display_name_i18n_key"] = strings.TrimSpace(pack.DisplayNameI18nKey)
	metadata["description_i18n_key"] = strings.TrimSpace(pack.DescriptionI18nKey)
	metadata["owner_scope"] = strings.TrimSpace(pack.OwnerScope)
	metadata["required_node_kinds"] = pack.RequiredNodeKinds
	metadata["required_capabilities"] = pack.RequiredCapabilities
	return metadata
}

func normalizeKeyFilter(keys []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, key := range keys {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			out[trimmed] = struct{}{}
		}
	}
	return out
}
