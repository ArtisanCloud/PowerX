package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	agentcfg "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	intentfactory "github.com/ArtisanCloud/PowerX/internal/server/agent/factory/intent"
	agentsettings "github.com/ArtisanCloud/PowerX/internal/service/agent"
	knowledge "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type resolvedEmbeddingProfile struct {
	Env        string
	Provider   string
	Model      string
	Endpoint   string
	Dimensions int
}

func (s *IngestionService) resolveEmbeddingVectorizer(
	ctx context.Context,
	tenantUUID string,
) (*resolvedEmbeddingProfile, agentSvcEmbedVectorizer, error) {
	if s == nil || s.agentSettings == nil {
		return nil, nil, nil
	}
	tid := strings.ToLower(strings.TrimSpace(tenantUUID))
	if tid == "" {
		return nil, nil, fmt.Errorf("tenant_uuid is empty")
	}

	env, configured, err := s.agentSettings.GetTenantCurrentAIEnv(ctx, tid)
	if err != nil {
		// 测试/轻量环境可能没迁移 AI setting 表：此时回退默认 env，而不是直接失败。
		if !isMissingTableError(err) {
			return nil, nil, err
		}
		env = "dev"
		configured = false
	}
	if !configured {
		env = "dev"
	}

	profile, err := s.agentSettings.GetActiveProfile(ctx, env, &tid, "embedding")
	if err != nil {
		// 同上：若 AI profile/route 表不存在，则直接用 config.yaml 的 ai.defaults.embedding 兜底。
		if !isMissingTableError(err) {
			return nil, nil, err
		}
		profile = nil
	}
	if profile == nil {
		cfg := agentcfg.GetGlobalAIConfig()
		if cfg == nil {
			return nil, nil, nil
		}
		provider := strings.ToLower(strings.TrimSpace(cfg.Defaults.Embedding.Provider))
		model := strings.TrimSpace(cfg.Defaults.Embedding.Model)
		if provider == "" || model == "" || provider == "none" || provider == "disabled" {
			return nil, nil, nil
		}
		embCfg := agentcfg.EmbeddingConfig{
			Enabled:  true,
			Provider: provider,
			Endpoint: strings.TrimSpace(cfg.Defaults.Embedding.Endpoint),
			Model:    model,
			APIKey:   strings.TrimSpace(cfg.Defaults.Embedding.APIKey),
			MaxBatch: cfg.Defaults.Embedding.Batch,
			Dim:      cfg.Defaults.Embedding.Dimensions,
		}
		if strings.TrimSpace(embCfg.Endpoint) == "" {
			req := catalog.AuthReqFromCatalog(provider)
			embCfg.Endpoint = strings.TrimSpace(req.DefaultBaseURL)
		}
		vec, err := intentfactory.NewVectorizerFromConfig(embCfg)
		if err != nil {
			return &resolvedEmbeddingProfile{
				Env:        env,
				Provider:   provider,
				Model:      model,
				Endpoint:   embCfg.Endpoint,
				Dimensions: embCfg.Dim,
			}, nil, err
		}
		return &resolvedEmbeddingProfile{
			Env:        env,
			Provider:   provider,
			Model:      model,
			Endpoint:   embCfg.Endpoint,
			Dimensions: embCfg.Dim,
		}, vec, nil
	}

	provider := strings.ToLower(strings.TrimSpace(profile.Provider))
	model := strings.TrimSpace(profile.Model)
	if provider == "" || model == "" || provider == "none" || provider == "disabled" {
		return nil, nil, nil
	}
	return s.resolveEmbeddingVectorizerForProfile(ctx, tid, env, provider, model)
}

// resolveEmbeddingVectorizerForProfile resolves embedder for an explicit provider+model (space-locked use-case).
func (s *IngestionService) resolveEmbeddingVectorizerForProfile(
	ctx context.Context,
	tenantUUID string,
	env string,
	provider string,
	model string,
) (*resolvedEmbeddingProfile, agentSvcEmbedVectorizer, error) {
	if s == nil || s.agentSettings == nil {
		return nil, nil, nil
	}
	tid := strings.ToLower(strings.TrimSpace(tenantUUID))
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.TrimSpace(model)
	env = strings.TrimSpace(env)
	if tid == "" || env == "" || provider == "" || model == "" {
		return nil, nil, fmt.Errorf("invalid embedding profile params")
	}

	// best-effort read profile defaults (dims/base_url/api_key)
	var baseURLIn, apiKeyIn string
	dimensions := 0
	maxBatch := 0
	if prof, err := s.agentSettings.GetProfile(ctx, env, &tid, "embedding", provider, model); err == nil && prof != nil {
		if prof.Defaults != nil {
			if v, ok := prof.Defaults["endpoint"].(string); ok && strings.TrimSpace(v) != "" {
				baseURLIn = strings.TrimSpace(v)
			} else if v, ok := prof.Defaults["base_url"].(string); ok && strings.TrimSpace(v) != "" {
				baseURLIn = strings.TrimSpace(v)
			}
			if v, ok := prof.Defaults["api_key"].(string); ok && strings.TrimSpace(v) != "" {
				apiKeyIn = strings.TrimSpace(v)
			}
			if v, ok := prof.Defaults["batch"]; ok {
				maxBatch = parseInt(v)
			}
		}
		dimensions = agentsettings.ResolveEmbeddingDimensions(prof)
	}

	// If provider in catalog declares an embedding driver, use driverKey to pick implementation;
	// credentials still come from the original provider.
	driverKey := provider
	if m, ok := catalog.GetGlobalAIRegister().Manifest(provider); ok && m != nil {
		if dk := strings.ToLower(strings.TrimSpace(m.Drivers["embedding"])); dk != "" {
			driverKey = dk
		}
	}

	baseURL := baseURLIn
	apiKey := apiKeyIn
	if bu, ak, e := s.agentSettings.ResolveConnFromStore(ctx, env, &tid, provider, baseURLIn, apiKeyIn); e == nil {
		baseURL, apiKey = bu, ak
	} else if isMissingTableError(e) || errors.Is(e, gorm.ErrRecordNotFound) {
		// ignore and fallback to baseURLIn/apiKeyIn/catalog defaults
	} else {
		return nil, nil, e
	}
	if strings.TrimSpace(baseURL) == "" {
		if v := catalog.DefaultBaseURLForModel(provider, model); strings.TrimSpace(v) != "" {
			baseURL = v
		} else {
			req := catalog.AuthReqFromCatalog(provider)
			baseURL = strings.TrimSpace(req.DefaultBaseURL)
		}
	}

	// provider/driver 对 key 的要求来自 catalog；若需要 key 但缺失，则视为“未配置”而不是直接入库失败。
	// 注意：catalog 在某些测试/轻量环境可能未初始化；此时 AuthReqFromCatalog 会保守返回 NeedKey=true。
	// 对于明确“不需要 key”的内置 driver，我们做白名单豁免，避免误判。
	req := catalog.AuthReqFromCatalog(provider)
	if req.NeedKey && strings.TrimSpace(apiKey) == "" &&
		provider != "hash" &&
		provider != "openai_compatible" && provider != "openai-compatible" && provider != "openai_compat" &&
		provider != "ollama" &&
		provider != "sentence_transformers" && provider != "sentence-transformers" && provider != "sbert" {
		return &resolvedEmbeddingProfile{
			Env:        env,
			Provider:   provider,
			Model:      model,
			Endpoint:   baseURL,
			Dimensions: dimensions,
		}, nil, nil
	}

	embCfg := agentcfg.EmbeddingConfig{
		Enabled:  true,
		Provider: driverKey,
		Endpoint: baseURL,
		Model:    model,
		APIKey:   apiKey,
		MaxBatch: maxBatch,
		Dim:      dimensions,
	}
	vec, err := intentfactory.NewVectorizerFromConfig(embCfg)
	if err != nil {
		return &resolvedEmbeddingProfile{
			Env:      env,
			Provider: provider,
			Model:    model,
			Endpoint: baseURL,
		}, nil, err
	}
	if vec == nil {
		return &resolvedEmbeddingProfile{
			Env:      env,
			Provider: provider,
			Model:    model,
			Endpoint: baseURL,
		}, nil, nil
	}

	return &resolvedEmbeddingProfile{
		Env:        env,
		Provider:   provider,
		Model:      model,
		Endpoint:   baseURL,
		Dimensions: dimensions,
	}, vec, nil
}

func (s *IngestionService) ensureEmbeddingReady(ctx context.Context, space *knowledge.KnowledgeSpace) error {
	if s == nil || s.agentSettings == nil {
		return dto.NewErrorWithCode(http.StatusPreconditionFailed, "embedding_not_configured", "AI Settings 未初始化，无法执行入库", ErrEmbeddingNotConfigured)
	}
	if space == nil || space.UUID == uuid.Nil {
		return ErrSpaceNotFound
	}
	profileKey := strings.TrimSpace(space.EmbeddingProfileKey)
	if profileKey == "" {
		return dto.NewErrorWithCode(http.StatusPreconditionFailed, "embedding_not_configured", "请先在 AI Settings 配置 embedding 模型并完成测试", ErrEmbeddingNotConfigured)
	}
	provider, model, err := ParseEmbeddingProfileKey(profileKey)
	if err != nil {
		return dto.NewErrorWithCode(http.StatusPreconditionFailed, "embedding_not_configured", "embedding profile 配置无效，请重新设置", ErrEmbeddingNotConfigured)
	}
	tenantUUID := strings.ToLower(strings.TrimSpace(space.TenantUUID))
	if tenantUUID == "" {
		return dto.NewErrorWithCode(http.StatusPreconditionFailed, "embedding_not_configured", "缺少租户上下文，无法确认 embedding 配置", ErrEmbeddingNotConfigured)
	}
	env, _, err := s.agentSettings.GetTenantCurrentAIEnv(ctx, tenantUUID)
	if err != nil && !isMissingTableError(err) {
		return dto.NewErrorWithCode(http.StatusInternalServerError, "embedding_failed", "获取 AI 环境失败", err)
	}
	if strings.TrimSpace(env) == "" {
		env = "dev"
	}
	profile, err := s.agentSettings.GetProfile(ctx, env, &tenantUUID, "embedding", provider, model)
	if err != nil {
		if isMissingTableError(err) || errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.NewErrorWithCode(http.StatusPreconditionFailed, "embedding_not_configured", "请先在 AI Settings 配置 embedding 模型并完成测试", ErrEmbeddingNotConfigured)
		}
		return dto.NewErrorWithCode(http.StatusInternalServerError, "embedding_failed", "读取 embedding 配置失败", err)
	}
	if profile == nil {
		return dto.NewErrorWithCode(http.StatusPreconditionFailed, "embedding_not_configured", "请先在 AI Settings 配置 embedding 模型并完成测试", ErrEmbeddingNotConfigured)
	}
	if !agentsettings.EmbeddingProfileReady(profile) {
		return dto.NewErrorWithCode(http.StatusPreconditionFailed, "embedding_probe_required", "embedding 模型未完成测试探测，请先在 AI Settings 执行测试", ErrEmbeddingNotConfigured)
	}
	_, vec, err := s.resolveEmbeddingVectorizerForProfile(ctx, tenantUUID, env, provider, model)
	if err != nil {
		return dto.NewErrorWithCode(http.StatusInternalServerError, "embedding_failed", "embedding 配置解析失败", err)
	}
	if vec == nil {
		return dto.NewErrorWithCode(http.StatusPreconditionFailed, "embedding_not_configured", "embedding 凭据未配置或不可用，请先在 AI Settings 完成配置", ErrEmbeddingNotConfigured)
	}
	return nil
}

// agentSvcEmbedVectorizer keeps knowledge_space decoupled from internal/server/agent embed package.
// It mirrors `internal/server/agent/contract/embed.Vectorizer`.
type agentSvcEmbedVectorizer interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

func parseInt(v any) int {
	switch val := v.(type) {
	case float64:
		if int(val) > 0 {
			return int(val)
		}
	case float32:
		if int(val) > 0 {
			return int(val)
		}
	case int:
		if val > 0 {
			return val
		}
	case int32:
		if val > 0 {
			return int(val)
		}
	case int64:
		if val > 0 {
			return int(val)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && parsed > 0 {
			return parsed
		}
	case json.Number:
		if parsed, err := val.Int64(); err == nil && parsed > 0 {
			return int(parsed)
		}
		if parsed, err := strconv.Atoi(strings.TrimSpace(val.String())); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func (s *IngestionService) buildVectorRecords(
	ctx context.Context,
	space *knowledge.KnowledgeSpace,
	chunks []IngestionChunk,
) (records []vectorstore.VectorRecord, embeddingPct float64, degraded bool, errorCode string, reason string) {
	if len(chunks) == 0 {
		return nil, 0, false, "", ""
	}
	if space == nil || space.UUID == uuid.Nil {
		return nil, 0, true, "space_not_found", "space_missing"
	}

	// 对所有非空 chunk 做向量化（doc_summary/section_summary/chunk），保持“全链路可观测”一致性；
	// 检索侧若只想用正文 chunk，可通过 metadata.filter（chunk_kind）筛选。
	contentIdx := make([]int, 0, len(chunks))
	texts := make([]string, 0, len(chunks))
	for i := range chunks {
		if strings.TrimSpace(chunks[i].Content) == "" {
			continue
		}
		contentIdx = append(contentIdx, i)
		texts = append(texts, chunks[i].Content)
	}
	if len(texts) == 0 {
		return nil, 0, true, "embedding_failed", "no_chunk_text"
	}

	// 没有向量存储 → 直接跳过，避免“看起来 100% 成功但实际上没写入”的假象。
	if s == nil || s.vectorStore == nil {
		return nil, 0, true, "vector_store_disabled", "vector_store_not_configured"
	}

	// Space 未激活 dense index：不进行 embedding（避免额外成本），直接降级。
	activeIndexKey := strings.TrimSpace(space.ActiveVectorIndexKey)
	if activeIndexKey == "" {
		return nil, 0, true, "vector_index_not_activated", "no_active_vector_index"
	}

	// Space 级锁定 embedding profile（provider/model）。
	profileKey := strings.TrimSpace(space.EmbeddingProfileKey)
	if profileKey == "" {
		return nil, 0, true, "embedding_not_configured", "space_embedding_profile_not_set"
	}
	provider, model, err := ParseEmbeddingProfileKey(profileKey)
	if err != nil {
		return nil, 0, true, "embedding_not_configured", err.Error()
	}

	tenantUUID := strings.ToLower(strings.TrimSpace(space.TenantUUID))
	if tenantUUID == "" {
		return nil, 0, true, "embedding_failed", "tenant_uuid_empty"
	}
	env, configured, err := s.agentSettings.GetTenantCurrentAIEnv(ctx, tenantUUID)
	if err != nil {
		if !isMissingTableError(err) {
			return nil, 0, true, "embedding_failed", fmt.Sprintf("resolve_env_failed: %v", err)
		}
		env = "dev"
		configured = false
	}
	if !configured || strings.TrimSpace(env) == "" {
		env = "dev"
	}

	// Load active index for dimension check.
	activeRec, err := repo.NewKnowledgeVectorIndexRepository(s.db).FindBySpaceAndKey(ctx, space.UUID, activeIndexKey)
	if err != nil {
		return nil, 0, true, "vector_index_invalid", fmt.Sprintf("load_active_index_failed: %v", err)
	}
	if activeRec == nil || activeRec.Dimensions <= 0 {
		return nil, 0, true, "vector_index_invalid", "active_index_not_found"
	}

	prof, vec, err := s.resolveEmbeddingVectorizerForProfile(ctx, tenantUUID, env, provider, model)
	if err != nil {
		return nil, 0, true, "embedding_failed", fmt.Sprintf("resolve_embedder_failed: %v", err)
	}
	if vec == nil {
		return nil, 0, true, "embedding_not_configured", "no_active_embedding_profile"
	}

	start := time.Now()
	embeddings, err := vec.Embed(ctx, texts)
	latency := time.Since(start)
	if err != nil {
		return nil, 0, true, "embedding_failed", fmt.Sprintf("embed_failed: %v", err)
	}
	if len(embeddings) != len(texts) {
		return nil, 0, true, "embedding_failed", fmt.Sprintf("embed_failed: batch_mismatch (want=%d got=%d)", len(texts), len(embeddings))
	}

	expectedDim := activeRec.Dimensions
	for i := range embeddings {
		if len(embeddings[i]) != expectedDim {
			return nil, 0, true, "embedding_dim_mismatch",
				fmt.Sprintf("embedding_dim=%d != active_pgvector_dim=%d (provider=%s model=%s index_key=%s)",
					len(embeddings[i]), expectedDim, prof.Provider, prof.Model, activeIndexKey)
		}
	}

	records = make([]vectorstore.VectorRecord, 0, len(texts))
	for pos, idx := range contentIdx {
		chunk := chunks[idx]
		meta := make(map[string]any, len(chunk.Metadata)+6)
		for k, v := range chunk.Metadata {
			meta[k] = v
		}
		meta["chunk_kind"] = chunk.Kind
		meta["content_hash"] = ContentHash(chunk.Content)
		meta["embedding_provider"] = prof.Provider
		meta["embedding_model"] = prof.Model
		meta["embedding_env"] = prof.Env
		meta["embedding_profile_ref"] = profileKey
		meta["active_vector_index_key"] = activeIndexKey
		meta["embedding_latency_ms"] = latency.Milliseconds()

		records = append(records, vectorstore.VectorRecord{
			ChunkID:   chunk.ID,
			Embedding: embeddings[pos],
			Metadata:  meta,
		})
	}

	embeddingPct = 100.0 * float64(len(records)) / float64(len(texts))
	return records, embeddingPct, false, "", ""
}
