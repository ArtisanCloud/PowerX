package knowledge_space

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	agentsettings "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
)

const (
	BuiltinKnowledgeSpaceName         = "插件联调知识空间"
	builtinKnowledgeDepartmentCode    = "CORE"
	builtinKnowledgePolicyTemplate    = "default"
	builtinKnowledgePolicyVersion     = "v1"
	builtinKnowledgeEmbeddingEnv      = "dev"
	builtinKnowledgeEmbeddingProvider = "hash"
	builtinKnowledgeEmbeddingModel    = "hash"
	builtinKnowledgeEmbeddingDims     = 32
	builtinKnowledgeEmbeddingKey      = "hash/hash"
	builtinKnowledgeActor             = "builtin-knowledge-seed"
)

var ErrBuiltinKnowledgeSeedUnavailable = errors.New("knowledge.builtin_seed_unavailable")

type BuiltinKnowledgeInitializer struct {
	db      *gorm.DB
	spaces  *Service
	vectors *VectorIndexService
}

type BuiltinKnowledgeInitializerOptions struct {
	DB           *gorm.DB
	SpaceService *Service
	VectorIndex  *VectorIndexService
}

func NewBuiltinKnowledgeInitializer(opts BuiltinKnowledgeInitializerOptions) *BuiltinKnowledgeInitializer {
	if opts.DB == nil {
		panic("builtin knowledge initializer requires db")
	}
	if opts.SpaceService == nil {
		opts.SpaceService = NewService(ServiceOptions{DB: opts.DB})
	}
	return &BuiltinKnowledgeInitializer{
		db:      opts.DB,
		spaces:  opts.SpaceService,
		vectors: opts.VectorIndex,
	}
}

type BuiltinKnowledgeSeedInput struct {
	TenantUUID  string
	RequestedBy string
}

type BuiltinKnowledgeSeedResult struct {
	Spaces  []*models.KnowledgeSpace `json:"spaces"`
	Created int                      `json:"created"`
	Updated int                      `json:"updated"`
	Skipped []string                 `json:"skipped"`
}

func (s *BuiltinKnowledgeInitializer) EnsureTenantBuiltinKnowledge(ctx context.Context, in BuiltinKnowledgeSeedInput) (*BuiltinKnowledgeSeedResult, error) {
	if s == nil || s.db == nil || s.spaces == nil || s.vectors == nil {
		return nil, ErrBuiltinKnowledgeSeedUnavailable
	}
	tenantUUID, err := normalizeTenantUUID(in.TenantUUID)
	if err != nil {
		return nil, err
	}
	actor := strings.TrimSpace(in.RequestedBy)
	if actor == "" {
		actor = builtinKnowledgeActor
	}

	if err := s.ensureBuiltinEmbeddingProfile(ctx, tenantUUID); err != nil {
		return nil, err
	}
	if err := s.ensureBuiltinKnowledgeProfiles(ctx, tenantUUID, actor); err != nil {
		return nil, err
	}
	policyID, err := s.requireDefaultPolicyTemplate(ctx)
	if err != nil {
		return nil, err
	}

	result := &BuiltinKnowledgeSeedResult{Spaces: make([]*models.KnowledgeSpace, 0, 1)}
	space, created, err := s.ensureBuiltinSpace(ctx, tenantUUID, policyID, actor)
	if err != nil {
		return nil, err
	}
	if created {
		result.Created++
	} else {
		result.Updated++
	}
	result.Spaces = append(result.Spaces, space)
	return result, nil
}

func (s *BuiltinKnowledgeInitializer) ensureBuiltinEmbeddingProfile(ctx context.Context, tenantUUID string) error {
	settings := agentsettings.NewAgentSettingService(s.db)
	if err := settings.SetTenantCurrentAIEnv(ctx, tenantUUID, builtinKnowledgeEmbeddingEnv); err != nil {
		return err
	}
	profile := &agentmodel.AIModelProfile{
		Modality: builtinKnowledgeEmbeddingProvider,
		Provider: builtinKnowledgeEmbeddingProvider,
		Model:    builtinKnowledgeEmbeddingModel,
		Label:    "PowerX builtin local embedding",
		Defaults: datatypes.JSONMap{
			"dimensions":       builtinKnowledgeEmbeddingDims,
			"max_input_tokens": 2048,
		},
		CapCache: datatypes.JSONMap{
			"dimensions": builtinKnowledgeEmbeddingDims,
			"probed_at":  time.Now().UTC().Format(time.RFC3339Nano),
		},
		Tags: []string{"embedding", "builtin", "local"},
	}
	profile.Modality = "embedding"
	if err := agentrepo.NewAIModelProfileRepository(s.db).
		UpsertByScopeModalityProviderModel(ctx, builtinKnowledgeEmbeddingEnv, &tenantUUID, profile); err != nil {
		return err
	}
	return agentrepo.NewAIRoutePolicyRepository(s.db).
		UpsertDefaultByScopeModality(ctx, builtinKnowledgeEmbeddingEnv, &tenantUUID, "embedding", builtinKnowledgeEmbeddingProvider, builtinKnowledgeEmbeddingModel)
}

func (s *BuiltinKnowledgeInitializer) ensureBuiltinKnowledgeProfiles(ctx context.Context, tenantUUID string, actor string) error {
	now := time.Now().UTC()
	items := []struct {
		key  string
		name string
		cfg  datatypes.JSON
	}{
		{key: "p0_basic", name: "P0 Basic", cfg: datatypes.JSON([]byte(`{"bundle":"p0_basic"}`))},
		{key: "p1_general", name: "P1 General", cfg: datatypes.JSON([]byte(`{"bundle":"p1_general"}`))},
	}
	for _, item := range items {
		if err := upsertBuiltinIngestionProfile(ctx, s.db, tenantUUID, item.key, item.name, item.cfg, actor, &now); err != nil {
			return err
		}
		if err := upsertBuiltinIndexProfile(ctx, s.db, tenantUUID, item.key, item.name, item.cfg, actor, &now); err != nil {
			return err
		}
		if err := upsertBuiltinRAGProfile(ctx, s.db, tenantUUID, item.key, item.name, item.cfg, actor, &now); err != nil {
			return err
		}
	}
	return nil
}

func (s *BuiltinKnowledgeInitializer) requireDefaultPolicyTemplate(ctx context.Context) (uint64, error) {
	policies := repo.NewPolicyTemplateRepository(s.db)
	tpl, err := policies.GetByNameVersion(ctx, builtinKnowledgePolicyTemplate, builtinKnowledgePolicyVersion)
	if err != nil {
		return 0, err
	}
	if tpl == nil || tpl.ID == 0 {
		return 0, dto.NewErrorWithCode(http.StatusPreconditionFailed, "knowledge.policy_template.default_v1_missing", "default/v1 policy template is required before seeding builtin knowledge spaces", ErrInvalidInput)
	}
	return tpl.ID, nil
}

func (s *BuiltinKnowledgeInitializer) ensureBuiltinSpace(ctx context.Context, tenantUUID string, policyID uint64, actor string) (*models.KnowledgeSpace, bool, error) {
	spaces := repo.NewKnowledgeSpaceRepository(s.db)
	existing, err := spaces.FindByTenantAndName(ctx, tenantUUID, BuiltinKnowledgeSpaceName)
	if err != nil {
		return nil, false, err
	}
	created := false
	var space *models.KnowledgeSpace
	flags := []string{
		"builtin:knowledge_space",
		"workflow.debug",
		"rag.scene:custom_expert",
		"rag.bundle:p0_basic",
		"rag.strategy_package:a_simple",
	}
	if existing == nil {
		space, err = s.spaces.CreateSpace(ctx, CreateSpaceInput{
			TenantUUID:          tenantUUID,
			SpaceName:           BuiltinKnowledgeSpaceName,
			DepartmentCode:      builtinKnowledgeDepartmentCode,
			QuotaCPU:            4,
			QuotaStorageGB:      120,
			PolicyVersion:       policyID,
			IngestionProfileKey: "p0_basic",
			IndexProfileKey:     "p0_basic",
			RAGProfileKey:       "p0_basic",
			FeatureFlags:        flags,
			RequestedBy:         actor,
		})
		if err != nil {
			return nil, false, err
		}
		created = true
	} else {
		if existing.Status == models.KnowledgeSpaceStatusRetired {
			return nil, false, dto.NewErrorWithCode(http.StatusConflict, "knowledge.builtin_space_retired", "builtin knowledge space is retired and cannot be reinitialized implicitly", ErrInvalidStatusTransition)
		}
		space, err = s.spaces.UpdateSpace(ctx, UpdateSpaceInput{
			SpaceID:             existing.UUID,
			QuotaCPU:            4,
			QuotaStorageGB:      120,
			PolicyVersion:       policyID,
			IngestionProfileKey: "p0_basic",
			IndexProfileKey:     "p0_basic",
			RAGProfileKey:       "p0_basic",
			FeatureFlags:        flags,
			UpdatedBy:           actor,
		})
		if err != nil {
			return nil, false, err
		}
	}

	if strings.TrimSpace(space.EmbeddingProfileKey) != builtinKnowledgeEmbeddingKey ||
		strings.TrimSpace(space.ActiveVectorIndexKey) == "" {
		activated, err := s.vectors.ActivateDenseIndex(ctx, ActivateDenseIndexInput{
			TenantUUID:          tenantUUID,
			SpaceUUID:           space.UUID,
			EmbeddingProfileKey: builtinKnowledgeEmbeddingKey,
			RequestedBy:         actor,
		})
		if err != nil {
			return nil, false, err
		}
		if activated != nil && activated.Space != nil {
			space = activated.Space
		}
	}
	if space.Status != models.KnowledgeSpaceStatusActive {
		space, err = s.spaces.UpdateSpace(ctx, UpdateSpaceInput{
			SpaceID:   space.UUID,
			Status:    models.KnowledgeSpaceStatusActive,
			UpdatedBy: actor,
		})
		if err != nil {
			return nil, false, err
		}
	}
	return space, created, nil
}

func upsertBuiltinIngestionProfile(ctx context.Context, db *gorm.DB, tenantUUID, key, name string, cfg datatypes.JSON, actor string, now *time.Time) error {
	var existing models.IngestionProfileVersion
	err := db.WithContext(ctx).
		Where("tenant_uuid = ? AND profile_key = ? AND status = ?", tenantUUID, key, models.ProfileStatusPublished).
		Order("version desc").
		Take(&existing).Error
	if err == nil && existing.ID > 0 {
		return db.WithContext(ctx).Model(&models.IngestionProfileVersion{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"display_name": name,
			"config":       cfg,
			"published_at": now,
			"published_by": actor,
		}).Error
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.WithContext(ctx).Create(&models.IngestionProfileVersion{
		TenantUUID: tenantUUID, ProfileKey: key, Version: 1, Status: models.ProfileStatusPublished,
		DisplayName: name, Config: cfg, PublishedAt: now, PublishedBy: actor, CreatedBy: actor,
	}).Error
}

func upsertBuiltinIndexProfile(ctx context.Context, db *gorm.DB, tenantUUID, key, name string, cfg datatypes.JSON, actor string, now *time.Time) error {
	var existing models.IndexProfileVersion
	err := db.WithContext(ctx).
		Where("tenant_uuid = ? AND profile_key = ? AND status = ?", tenantUUID, key, models.ProfileStatusPublished).
		Order("version desc").
		Take(&existing).Error
	if err == nil && existing.ID > 0 {
		return db.WithContext(ctx).Model(&models.IndexProfileVersion{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"display_name": name,
			"config":       cfg,
			"published_at": now,
			"published_by": actor,
		}).Error
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.WithContext(ctx).Create(&models.IndexProfileVersion{
		TenantUUID: tenantUUID, ProfileKey: key, Version: 1, Status: models.ProfileStatusPublished,
		DisplayName: name, Config: cfg, PublishedAt: now, PublishedBy: actor, CreatedBy: actor,
	}).Error
}

func upsertBuiltinRAGProfile(ctx context.Context, db *gorm.DB, tenantUUID, key, name string, cfg datatypes.JSON, actor string, now *time.Time) error {
	var existing models.RAGProfileVersion
	err := db.WithContext(ctx).
		Where("tenant_uuid = ? AND profile_key = ? AND status = ?", tenantUUID, key, models.ProfileStatusPublished).
		Order("version desc").
		Take(&existing).Error
	if err == nil && existing.ID > 0 {
		return db.WithContext(ctx).Model(&models.RAGProfileVersion{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"display_name": name,
			"config":       cfg,
			"published_at": now,
			"published_by": actor,
		}).Error
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.WithContext(ctx).Create(&models.RAGProfileVersion{
		TenantUUID: tenantUUID, ProfileKey: key, Version: 1, Status: models.ProfileStatusPublished,
		DisplayName: name, Config: cfg, PublishedAt: now, PublishedBy: actor, CreatedBy: actor,
	}).Error
}

func FormatBuiltinKnowledgeSeedSummary(result *BuiltinKnowledgeSeedResult) string {
	if result == nil {
		return "spaces=0 created=0 updated=0"
	}
	return fmt.Sprintf("spaces=%d created=%d updated=%d", len(result.Spaces), result.Created, result.Updated)
}
