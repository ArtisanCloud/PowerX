package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	agentsvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type SemanticRuntime struct {
	db            *gorm.DB
	agents        *agentsvc.AgentSettingService
	vectors       *RoutedVectorStore
	vectorWriter  vectorstore.Store
	embedOverride func(context.Context, string, string) (*resolvedEmbeddingProfile, agentSvcEmbedVectorizer, string, string, error)
}

func NewSemanticRuntime(db *gorm.DB, agents *agentsvc.AgentSettingService, vectors vectorstore.Store) *SemanticRuntime {
	routed, _ := vectors.(*RoutedVectorStore)
	return &SemanticRuntime{db: db, agents: agents, vectors: routed, vectorWriter: vectors}
}

func semanticError(status int, code string) error {
	return knowledgeError(status, code, errors.New(code))
}

// resolveModel 从真实已配置模型解析执行器；哈希执行器不能进入语义合同。
func (s *SemanticRuntime) resolveModel(ctx context.Context, tenant, key string) (*resolvedEmbeddingProfile, agentSvcEmbedVectorizer, string, string, error) {
	if s.embedOverride != nil {
		return s.embedOverride(ctx, tenant, key)
	}
	provider, model, err := ParseEmbeddingProfileKey(key)
	if err != nil || provider == "hash" {
		return nil, nil, "", "", semanticError(422, "KNOWLEDGE_EMBEDDING_NOT_CONFIGURED")
	}
	env := reqctx.GetEnv(ctx)
	if env == "" {
		env = "dev"
	}
	if s.agents == nil {
		return nil, nil, "", "", semanticError(503, "KNOWLEDGE_EMBEDDING_UNAVAILABLE")
	}
	profile, err := s.agents.GetProfile(ctx, env, &tenant, "embedding", provider, model)
	if err != nil || profile == nil {
		return nil, nil, "", "", semanticError(422, "KNOWLEDGE_EMBEDDING_NOT_CONFIGURED")
	}
	resolved, vec, err := (&IngestionService{agentSettings: s.agents}).resolveEmbeddingVectorizerForProfile(ctx, tenant, env, provider, model)
	if err != nil || vec == nil || resolved == nil {
		return nil, nil, "", "", semanticError(503, "KNOWLEDGE_EMBEDDING_UNAVAILABLE")
	}
	config := map[string]any{"profile_uuid": fmt.Sprint(profile.ID), "provider": provider, "model": model, "endpoint": resolved.Endpoint, "dimensions": resolved.Dimensions, "max_input_tokens": resolved.MaxInputTokens}
	configBytes, _ := json.Marshal(config)
	revision := ""
	if provider == "ollama" {
		endpoint, e := url.Parse(resolved.Endpoint)
		if e != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
			return nil, nil, "", "", semanticError(422, "KNOWLEDGE_EMBEDDING_NOT_CONFIGURED")
		}
		endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/api/tags"
		rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(rctx, http.MethodGet, endpoint.String(), nil)
		resp, e := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if e != nil {
			return nil, nil, "", "", semanticError(503, "KNOWLEDGE_MODEL_IDENTITY_UNAVAILABLE")
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, nil, "", "", semanticError(503, "KNOWLEDGE_MODEL_IDENTITY_UNAVAILABLE")
		}
		var listing struct {
			Models []struct {
				Name   string `json:"name"`
				Model  string `json:"model"`
				Digest string `json:"digest"`
			} `json:"models"`
		}
		if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&listing) != nil {
			return nil, nil, "", "", semanticError(503, "KNOWLEDGE_MODEL_IDENTITY_UNAVAILABLE")
		}
		wanted := model
		if !strings.Contains(wanted, ":") {
			wanted += ":latest"
		}
		for _, item := range listing.Models {
			if item.Name == wanted || item.Model == wanted {
				revision = item.Digest
				break
			}
		}
	} else if value, ok := profile.Defaults["model_revision"].(string); ok {
		revision = strings.TrimSpace(value)
	}
	if revision == "" {
		return nil, nil, "", "", semanticError(422, "KNOWLEDGE_MODEL_REVISION_REQUIRED")
	}
	return resolved, vec, revision, checksumBytes(configBytes), nil
}

type SemanticConfigureInput struct {
	EmbeddingProfileKey             string `json:"embedding_profile_key"`
	ExpectedConfigurationGeneration string `json:"expected_configuration_generation,omitempty"`
}

func (s *SemanticRuntime) Configure(ctx context.Context, tenant, space string, input SemanticConfigureInput) (SemanticGeneration, error) {
	var row models.KnowledgeSpace
	if s.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ? AND status <> ?", tenant, space, models.KnowledgeSpaceStatusRetired).First(&row).Error != nil {
		return SemanticGeneration{}, KnowledgeSpaceNotFoundError(errors.New("space unavailable"))
	}
	if input.EmbeddingProfileKey == "" || row.EmbeddingProfileKey != input.EmbeddingProfileKey {
		return SemanticGeneration{}, semanticError(409, "KNOWLEDGE_MODEL_BINDING_CONFLICT")
	}
	resolved, embed, revision, configHash, err := s.resolveModel(ctx, tenant, input.EmbeddingProfileKey)
	if err != nil {
		return SemanticGeneration{}, err
	}
	probe, err := embed.Embed(ctx, []string{"PowerX semantic index dimension probe"})
	if err != nil || len(probe) != 1 || len(probe[0]) == 0 {
		return SemanticGeneration{}, semanticError(503, "KNOWLEDGE_EMBEDDING_FAILED")
	}
	dim := len(probe[0])
	if resolved.Dimensions > 0 && resolved.Dimensions != dim {
		return SemanticGeneration{}, semanticError(409, "KNOWLEDGE_MODEL_DIMENSION_CONFLICT")
	}
	index, err := repo.NewKnowledgeVectorIndexRepository(s.db).FindBySpaceAndKey(ctx, row.UUID, row.ActiveVectorIndexKey)
	if err != nil || index == nil || index.Dimensions != dim || s.vectors == nil || s.vectors.Driver() != "pgvector" {
		return SemanticGeneration{}, semanticError(422, "KNOWLEDGE_SEMANTIC_INDEX_NOT_READY")
	}
	store, _, err := s.vectors.resolveActiveStore(ctx, row.UUID)
	if err != nil || store == nil || store.Health(ctx) != nil {
		return SemanticGeneration{}, semanticError(503, "KNOWLEDGE_VECTOR_STORE_UNAVAILABLE")
	}
	var binding models.SemanticSpaceBinding
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := NewHostContractService(tx).lockHostSpace(ctx, tx, tenant, space); err != nil {
			return err
		}
		err := tx.Where("tenant_uuid = ? AND space_uuid = ?", tenant, space).First(&binding).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if input.ExpectedConfigurationGeneration != "" && binding.ConfigurationGeneration != input.ExpectedConfigurationGeneration {
			return semanticError(409, "KNOWLEDGE_INDEX_GENERATION_CONFLICT")
		}
		var profile models.SemanticEmbeddingProfile
		if binding.UUID != uuid.Nil {
			if tx.Where("tenant_uuid = ? AND uuid = ?", tenant, binding.EmbeddingProfileUUID).First(&profile).Error != nil {
				return semanticError(422, "KNOWLEDGE_SEMANTIC_INDEX_NOT_READY")
			}
			if profile.ConfigChecksum == configHash && profile.ModelRevision == revision && profile.Dimensions == dim && binding.VectorIndexKey == row.ActiveVectorIndexKey {
				return nil
			}
		}
		var version int
		if err := tx.Model(&models.SemanticEmbeddingProfile{}).Where("tenant_uuid = ? AND profile_key = ?", tenant, input.EmbeddingProfileKey).Select("COALESCE(MAX(version),0)").Scan(&version).Error; err != nil {
			return err
		}
		provider, model, _ := ParseEmbeddingProfileKey(input.EmbeddingProfileKey)
		profile = models.SemanticEmbeddingProfile{TenantUUID: tenant, ProfileKey: input.EmbeddingProfileKey, Version: version + 1, Status: models.ProfileStatusPublished, Provider: provider, Model: model, ModelRevision: revision, Dimensions: dim, ConfigChecksum: configHash}
		if err := tx.Create(&profile).Error; err != nil {
			return err
		}
		binding.TenantUUID, binding.SpaceUUID, binding.EmbeddingProfileUUID = tenant, space, fmt.Sprint(profile.ID)
		binding.ConfigurationGeneration, binding.CorpusGeneration, binding.VectorIndexKey = uuid.NewString(), uuid.NewString(), row.ActiveVectorIndexKey
		binding.Modes = datatypes.JSON(`["semantic","hybrid"]`)
		return tx.Save(&binding).Error
	})
	if err != nil {
		return SemanticGeneration{}, err
	}
	return s.generation(ctx, binding)
}

func (s *SemanticRuntime) generation(ctx context.Context, b models.SemanticSpaceBinding) (SemanticGeneration, error) {
	var p models.SemanticEmbeddingProfile
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", b.TenantUUID, b.EmbeddingProfileUUID).First(&p).Error; err != nil {
		return SemanticGeneration{}, err
	}
	return SemanticGeneration{SpaceUUID: b.SpaceUUID, ConfigurationGeneration: b.ConfigurationGeneration, CorpusGeneration: b.CorpusGeneration, EmbeddingProfile: HostTaskProfileRef{UUID: p.UUID.String(), Version: p.Version}, ModelKey: p.ProfileKey, ModelRevision: p.ModelRevision, Dimensions: p.Dimensions, DistanceMetric: "cosine"}, nil
}
