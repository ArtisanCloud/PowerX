package knowledge_space

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type HostQuotas struct {
	CPUCores             int `json:"cpu_cores"`
	StorageGB            int `json:"storage_gb"`
	IngestionConcurrency int `json:"ingestion_concurrency"`
}
type HostProfileRef struct {
	UUID    string `json:"uuid"`
	Key     string `json:"key"`
	Version int    `json:"version"`
}
type HostProfileMapping struct {
	Ingestion *HostProfileRef `json:"ingestion,omitempty"`
	Index     *HostProfileRef `json:"index,omitempty"`
	RAG       *HostProfileRef `json:"rag,omitempty"`
}
type HostPolicyTemplate struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Version string `json:"version"`
}
type HostStrategyPackage struct {
	UUID                   string              `json:"uuid"`
	Version                int                 `json:"version"`
	Key                    string              `json:"key"`
	Label                  string              `json:"label"`
	Summary                string              `json:"summary"`
	RecommendedProfileKey  string              `json:"recommended_profile_key"`
	RecommendedScenes      []string            `json:"recommended_scenes"`
	Dependencies           map[string][]string `json:"dependencies"`
	Profiles               HostProfileMapping  `json:"profiles"`
	Available              bool                `json:"available"`
	UnavailableReasons     []string            `json:"unavailable_reasons"`
	ActivationDependencies []string            `json:"activation_dependencies"`
}
type HostScene struct {
	Key            string   `json:"key"`
	Label          string   `json:"label"`
	Description    string   `json:"description"`
	DefaultBundle  string   `json:"default_bundle"`
	AllowedBundles []string `json:"allowed_bundles"`
}
type HostCatalog struct {
	DocumentIngestion         map[string]any        `json:"document_ingestion"`
	Version                   string                `json:"version"`
	Source                    string                `json:"source"`
	Scenes                    []HostScene           `json:"scenes"`
	StrategyPackages          []HostStrategyPackage `json:"strategy_packages"`
	PolicyTemplates           []HostPolicyTemplate  `json:"policy_templates"`
	DefaultPolicyTemplateUUID string                `json:"default_policy_template_uuid,omitempty"`
	QuotaDefaults             HostQuotas            `json:"quota_defaults"`
	QuotaMinimums             HostQuotas            `json:"quota_minimums"`
	QuotaOverrideAllowed      bool                  `json:"quota_override_allowed"`
}

// Catalog reads the Core catalog and real published tenant profiles. It never
// creates profiles/templates or substitutes a frontend/local catalog.
func (s *Service) GetHostCatalog(ctx context.Context, tenant string) (*HostCatalog, error) {
	if s == nil || s.db == nil || s.strategyCatalog == nil {
		return nil, KnowledgeUpstreamDependencyError(errors.New("knowledge catalog unavailable"))
	}
	cat, err := s.strategyCatalog.Load()
	if err != nil {
		return nil, KnowledgeUpstreamDependencyError(err)
	}
	out := &HostCatalog{DocumentIngestion: HostDocumentIngestionCapabilities(), Version: strconv.Itoa(cat.Version), Source: "powerx_core", Scenes: []HostScene{}, StrategyPackages: []HostStrategyPackage{}, PolicyTemplates: []HostPolicyTemplate{}, QuotaDefaults: HostQuotas{4, 200, 2}, QuotaMinimums: HostQuotas{1, 50, 1}, QuotaOverrideAllowed: true}
	var policies []models.PolicyTemplateVersion
	if err := s.db.WithContext(ctx).Order("template_name, version").Find(&policies).Error; err != nil {
		return nil, KnowledgeUpstreamDependencyError(err)
	}
	for _, policy := range policies {
		if policy.UUID == nil || *policy.UUID == uuid.Nil {
			return nil, KnowledgeUpstreamDependencyError(errors.New("policy template UUID migration required"))
		}
		out.PolicyTemplates = append(out.PolicyTemplates, HostPolicyTemplate{policy.UUID.String(), policy.TemplateName, policy.Version})
		if policy.TemplateName == "default" && policy.Version == "v1" {
			out.DefaultPolicyTemplateUUID = policy.UUID.String()
		}
	}
	for key, scene := range cat.Scenes {
		out.Scenes = append(out.Scenes, HostScene{key, scene.Label, scene.Description, scene.DefaultBundle, scene.AllowedBundles})
	}
	sort.Slice(out.Scenes, func(i, j int) bool { return out.Scenes[i].Key < out.Scenes[j].Key })
	embeddingErr := s.ensureTenantEmbeddingConfigured(ctx, tenant)
	if embeddingErr != nil && !errors.Is(embeddingErr, ErrEmbeddingNotConfigured) {
		return nil, embeddingErr
	}
	caps := computeStrategyCapabilities()
	for key, p := range cat.StrategyPackages {
		mapping, err := s.hostProfileMapping(ctx, tenant, p.RecommendedProfileKey)
		if err != nil {
			return nil, err
		}
		item := HostStrategyPackage{UUID: hostStrategyUUID(key, cat.Version), Version: cat.Version, Key: key, Label: p.Label, Summary: p.Summary, RecommendedProfileKey: p.RecommendedProfileKey, RecommendedScenes: p.RecommendedScenes, Dependencies: map[string][]string{"index": append([]string{}, p.Dependencies.Index...), "runtime": append([]string{}, p.Dependencies.Runtime...), "assets": append([]string{}, p.Dependencies.Assets...)}, Profiles: mapping, UnavailableReasons: []string{}, ActivationDependencies: append(append([]string{}, p.Dependencies.Index...), p.Dependencies.Assets...)}
		if mapping.Ingestion == nil {
			item.UnavailableReasons = append(item.UnavailableReasons, "ingestion_profile_not_published")
		}
		if mapping.Index == nil {
			item.UnavailableReasons = append(item.UnavailableReasons, "index_profile_not_published")
		}
		if mapping.RAG == nil {
			item.UnavailableReasons = append(item.UnavailableReasons, "rag_profile_not_published")
		}
		if embeddingErr != nil {
			item.UnavailableReasons = append(item.UnavailableReasons, "embedding_not_configured")
		}
		if len(out.PolicyTemplates) == 0 {
			item.UnavailableReasons = append(item.UnavailableReasons, "policy_template_missing")
		}
		for _, dependency := range p.Dependencies.Runtime {
			capKey := dependency
			if !strings.HasPrefix(capKey, "runtime.") {
				capKey = "runtime." + capKey
			}
			if !caps[capKey] {
				item.UnavailableReasons = append(item.UnavailableReasons, "runtime_dependency_unavailable:"+dependency)
			}
		}
		item.Available = len(item.UnavailableReasons) == 0
		out.StrategyPackages = append(out.StrategyPackages, item)
	}
	sort.Slice(out.StrategyPackages, func(i, j int) bool { return out.StrategyPackages[i].Key < out.StrategyPackages[j].Key })
	return out, nil
}

func (s *Service) hostProfileMapping(ctx context.Context, tenant, key string) (HostProfileMapping, error) {
	var out HostProfileMapping
	for _, entry := range []struct {
		model  any
		target **HostProfileRef
	}{
		{&models.IngestionProfileVersion{}, &out.Ingestion}, {&models.IndexProfileVersion{}, &out.Index}, {&models.RAGProfileVersion{}, &out.RAG},
	} {
		var row struct {
			UUID       uuid.UUID
			ProfileKey string
			Version    int
		}
		err := s.db.WithContext(ctx).Model(entry.model).Select("uuid, profile_key, version").Where("tenant_uuid = ? AND profile_key = ? AND status = ?", tenant, key, models.ProfileStatusPublished).Order("version DESC").Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return out, KnowledgeUpstreamDependencyError(err)
		}
		if row.UUID == uuid.Nil {
			return out, KnowledgeUpstreamDependencyError(errors.New("profile UUID missing"))
		}
		*entry.target = &HostProfileRef{row.UUID.String(), row.ProfileKey, row.Version}
	}
	return out, nil
}

type HostCreateSpaceRequest struct {
	Name                 string      `json:"name"`
	DepartmentUUID       string      `json:"department_uuid"`
	StrategyKey          string      `json:"strategy_key"`
	SceneKey             string      `json:"scene_key,omitempty"`
	PolicyTemplateUUID   string      `json:"policy_template_uuid,omitempty"`
	IngestionProfileUUID string      `json:"ingestion_profile_uuid,omitempty"`
	IndexProfileUUID     string      `json:"index_profile_uuid,omitempty"`
	RAGProfileUUID       string      `json:"rag_profile_uuid,omitempty"`
	Quotas               *HostQuotas `json:"quotas,omitempty"`
}
type HostCreatedSpace struct {
	SpaceUUID          string             `json:"space_uuid"`
	Name               string             `json:"name"`
	Status             string             `json:"status"`
	DepartmentUUID     string             `json:"department_uuid"`
	StrategyKey        string             `json:"strategy_key"`
	SceneKey           string             `json:"scene_key"`
	PolicyTemplateUUID string             `json:"policy_template_uuid"`
	Profiles           HostProfileMapping `json:"profiles"`
	Quotas             HostQuotas         `json:"quotas"`
}

func (s *Service) CreateHostSpace(ctx context.Context, tenant string, in HostCreateSpaceRequest) (*HostCreatedSpace, error) {
	if s == nil || s.db == nil {
		return nil, KnowledgeUpstreamDependencyError(errors.New("knowledge provisioning unavailable"))
	}
	departmentID, err := uuid.Parse(in.DepartmentUUID)
	if err != nil || departmentID == uuid.Nil || strings.TrimSpace(in.Name) == "" || len(in.Name) > 128 {
		return nil, KnowledgeInvalidArgumentError(errors.New("name and department_uuid are required"))
	}
	var department iam.Department
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND department_uuid = ? AND status = ?", tenant, departmentID, 1).First(&department).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, KnowledgeForbiddenError(errors.New("department not available in current tenant"))
		}
		return nil, KnowledgeUpstreamDependencyError(err)
	}
	cat, err := s.GetHostCatalog(ctx, tenant)
	if err != nil {
		return nil, err
	}
	var strategy *HostStrategyPackage
	for index := range cat.StrategyPackages {
		if cat.StrategyPackages[index].Key == in.StrategyKey {
			strategy = &cat.StrategyPackages[index]
			break
		}
	}
	if strategy == nil {
		return nil, KnowledgeInvalidArgumentError(errors.New("unknown strategy_key"))
	}
	if !strategy.Available {
		return nil, dto.NewErrorWithCode(http.StatusPreconditionFailed, "KNOWLEDGE_STRATEGY_UNAVAILABLE", strings.Join(strategy.UnavailableReasons, ","), ErrStrategyPrereqFailed)
	}
	scene := in.SceneKey
	if scene == "" && len(strategy.RecommendedScenes) > 0 {
		scene = strategy.RecommendedScenes[0]
	}
	if !knowledgeContains(strategy.RecommendedScenes, scene) {
		return nil, KnowledgeInvalidArgumentError(errors.New("scene is not supported by the strategy"))
	}
	profiles := strategy.Profiles
	for _, selection := range []struct {
		requested string
		resolved  *HostProfileRef
	}{
		{in.IngestionProfileUUID, profiles.Ingestion}, {in.IndexProfileUUID, profiles.Index}, {in.RAGProfileUUID, profiles.RAG},
	} {
		if selection.requested != "" && selection.requested != selection.resolved.UUID {
			return nil, KnowledgeInvalidArgumentError(errors.New("profile UUID must match the current catalog mapping"))
		}
	}
	policyUUID := in.PolicyTemplateUUID
	if policyUUID == "" {
		policyUUID = cat.DefaultPolicyTemplateUUID
	}
	policyID, err := uuid.Parse(policyUUID)
	if err != nil || policyID == uuid.Nil {
		return nil, KnowledgeInvalidArgumentError(errors.New("select a policy_template_uuid from the catalog"))
	}
	var policy models.PolicyTemplateVersion
	if err := s.db.WithContext(ctx).Where("uuid = ?", policyID).First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, KnowledgeInvalidArgumentError(errors.New("unknown policy_template_uuid"))
		}
		return nil, KnowledgeUpstreamDependencyError(err)
	}
	quotas := cat.QuotaDefaults
	if in.Quotas != nil {
		quotas = *in.Quotas
	}
	if quotas.CPUCores < 1 || quotas.StorageGB < 50 || quotas.IngestionConcurrency < 1 {
		return nil, KnowledgeInvalidArgumentError(errors.New("quotas are below catalog minimums"))
	}
	ingestionID, _ := uuid.Parse(profiles.Ingestion.UUID)
	indexID, _ := uuid.Parse(profiles.Index.UUID)
	ragID, _ := uuid.Parse(profiles.RAG.UUID)
	created, err := s.CreateSpace(ctx, CreateSpaceInput{TenantUUID: tenant, SpaceName: in.Name, DepartmentCode: department.Key, DepartmentUUID: &departmentID, PolicyVersion: policy.ID, QuotaCPU: quotas.CPUCores, QuotaStorageGB: quotas.StorageGB, IngestionProfileKey: profiles.Ingestion.Key, IndexProfileKey: profiles.Index.Key, RAGProfileKey: profiles.RAG.Key, IngestionProfileUUID: &ingestionID, IndexProfileUUID: &indexID, RAGProfileUUID: &ragID, FeatureFlags: EncodeConcurrencyFlag([]string{"rag.strategy_package:" + in.StrategyKey, "rag.strategy_version:" + cat.Version, "rag.scene:" + scene, "rag.bundle:" + profiles.RAG.Key}, quotas.IngestionConcurrency), RequestedBy: reqctx.GetSubject(ctx)})
	if err != nil {
		switch {
		case errors.Is(err, ErrSpaceConflict):
			return nil, dto.NewErrorWithCode(http.StatusConflict, "KNOWLEDGE_SPACE_CONFLICT", "space name already exists", err)
		case errors.Is(err, ErrInvalidInput):
			return nil, KnowledgeInvalidArgumentError(err)
		default:
			return nil, err
		}
	}
	return &HostCreatedSpace{created.UUID.String(), created.SpaceName, created.Status, departmentID.String(), in.StrategyKey, scene, policyUUID, profiles, quotas}, nil
}
