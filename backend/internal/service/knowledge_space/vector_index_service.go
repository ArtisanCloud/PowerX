package knowledge_space

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/migration"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	pgvectorcfg "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore/pgvector"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type VectorIndexService struct {
	db            *gorm.DB
	agentSettings *agentSvc.AgentSettingService
	pg            pgvectorcfg.Config
}

type VectorIndexServiceOptions struct {
	DB            *gorm.DB
	AgentSettings *agentSvc.AgentSettingService
	PGVector      pgvectorcfg.Config
}

func NewVectorIndexService(opts VectorIndexServiceOptions) *VectorIndexService {
	if opts.DB == nil {
		panic("vector index service requires db")
	}
	if opts.AgentSettings == nil {
		opts.AgentSettings = agentSvc.NewAgentSettingService(opts.DB)
	}
	return &VectorIndexService{
		db:            opts.DB,
		agentSettings: opts.AgentSettings,
		pg:            opts.PGVector.WithDefaults(),
	}
}

type VectorIndexStatus struct {
	SpaceID              string                      `json:"spaceId"`
	EmbeddingProfileKey   string                      `json:"embeddingProfileKey"`
	ActiveVectorIndexKey  string                      `json:"activeVectorIndexKey"`
	Active               *models.KnowledgeVectorIndex `json:"active,omitempty"`
	Indexes              []models.KnowledgeVectorIndex `json:"indexes"`
}

func (s *VectorIndexService) GetStatus(ctx context.Context, tenantUUID string, spaceUUID uuid.UUID, limit int) (*VectorIndexStatus, error) {
	if spaceUUID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	spaces := repo.NewKnowledgeSpaceRepository(s.db)
	space, err := spaces.FindByUUID(ctx, spaceUUID)
	if err != nil {
		return nil, err
	}
	if space == nil {
		return nil, ErrSpaceNotFound
	}
	if strings.ToLower(strings.TrimSpace(space.TenantUUID)) != strings.ToLower(strings.TrimSpace(tenantUUID)) {
		return nil, ErrSpaceNotFound
	}

	indexesRepo := repo.NewKnowledgeVectorIndexRepository(s.db)
	items, err := indexesRepo.ListBySpace(ctx, spaceUUID, limit)
	if err != nil {
		return nil, err
	}
	activeKey := strings.TrimSpace(space.ActiveVectorIndexKey)
	var active *models.KnowledgeVectorIndex
	if activeKey != "" {
		active, _ = indexesRepo.FindBySpaceAndKey(ctx, spaceUUID, activeKey)
	}
	return &VectorIndexStatus{
		SpaceID:             spaceUUID.String(),
		EmbeddingProfileKey:  strings.TrimSpace(space.EmbeddingProfileKey),
		ActiveVectorIndexKey: activeKey,
		Active:              active,
		Indexes:             items,
	}, nil
}

type ActivateDenseIndexInput struct {
	TenantUUID          string
	SpaceUUID           uuid.UUID
	EmbeddingProfileKey string
	RequestedBy         string
}

type ActivateDenseIndexResult struct {
	Space               *models.KnowledgeSpace       `json:"space"`
	ActiveIndex         *models.KnowledgeVectorIndex `json:"activeIndex"`
	CreatedTable        bool                         `json:"createdTable"`
}

func (s *VectorIndexService) ActivateDenseIndex(ctx context.Context, in ActivateDenseIndexInput) (*ActivateDenseIndexResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("service unavailable")
	}
	tid := strings.ToLower(strings.TrimSpace(in.TenantUUID))
	if tid == "" || in.SpaceUUID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	provider, model, err := ParseEmbeddingProfileKey(in.EmbeddingProfileKey)
	if err != nil {
		return nil, err
	}

	spaces := repo.NewKnowledgeSpaceRepository(s.db)
	indexes := repo.NewKnowledgeVectorIndexRepository(s.db)

	var out ActivateDenseIndexResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		spaceRepo := repo.NewKnowledgeSpaceRepository(tx)
		indexRepo := repo.NewKnowledgeVectorIndexRepository(tx)

		space, err := spaceRepo.FindByUUID(ctx, in.SpaceUUID)
		if err != nil {
			return err
		}
		if space == nil {
			return ErrSpaceNotFound
		}
		if strings.ToLower(strings.TrimSpace(space.TenantUUID)) != tid {
			return ErrSpaceNotFound
		}
		// 1) probe embedding dimensions (and validate credentials)
		dim, env, err := s.probeEmbeddingDimensions(ctx, tid, provider, model)
		if err != nil {
			return err
		}
		if dim <= 0 {
			return fmt.Errorf("embedding probe failed: dimensions=0 (provider=%s model=%s)", provider, model)
		}

		// 2) ensure pgvector table exists (idempotent)
		tableName := fmt.Sprintf("knowledge_vectors_v1_%d", dim)
		createdTable := false
		// best-effort existence check (no lock, just avoids extra Exec)
		var regclass *string
		_ = tx.Raw(`select to_regclass(?)`, fmt.Sprintf("%s.%s", s.pg.Schema, tableName)).Scan(&regclass).Error
			if regclass == nil || strings.TrimSpace(*regclass) == "" {
				dsn := strings.TrimSpace(s.pg.DSN)
				if dsn == "" {
					return fmt.Errorf("pgvector dsn is empty (configure knowledge_space.vector_store.pgvector.dsn or database.dsn)")
				}
				if err := migration.EnsureKnowledgeVectorsPGVectorTable(ctx, dsn, s.pg.Schema, tableName, dim, s.pg.Lists); err != nil {
					return err
				}
				createdTable = true
		}

		// 3) write index registry (allow multiple records for rollback)
		keySeed := fmt.Sprintf("%s|%s|%s|%s|%d|%s", tid, env, provider, model, dim, strings.TrimSpace(in.EmbeddingProfileKey))
		indexKey := fmt.Sprintf("dense_v1_%d_%s", dim, shortHash8(keySeed))

		now := time.Now()
		rec := &models.KnowledgeVectorIndex{
			SpaceUUID:           in.SpaceUUID,
			IndexKey:            indexKey,
			VectorTable:         tableName,
			Dimensions:          dim,
			EmbeddingProvider:   provider,
			EmbeddingModel:      model,
			EmbeddingProfileRef: strings.TrimSpace(in.EmbeddingProfileKey),
			Status:              models.KnowledgeVectorIndexStatusActive,
			LastUsedAt:          &now,
		}

		// retire old active indexes (best-effort)
		_ = tx.Model(&models.KnowledgeVectorIndex{}).
			Where("space_uuid = ? AND status = ? AND index_key <> ?", in.SpaceUUID, models.KnowledgeVectorIndexStatusActive, indexKey).
			Update("status", models.KnowledgeVectorIndexStatusRetired).Error

		exist, err := indexRepo.FindBySpaceAndKey(ctx, in.SpaceUUID, indexKey)
		if err != nil {
			return err
		}
		if exist == nil {
			if _, err := indexRepo.Create(ctx, rec); err != nil {
				return err
			}
		} else {
			exist.VectorTable = rec.VectorTable
			exist.Dimensions = rec.Dimensions
			exist.EmbeddingProvider = rec.EmbeddingProvider
			exist.EmbeddingModel = rec.EmbeddingModel
			exist.EmbeddingProfileRef = rec.EmbeddingProfileRef
			exist.Status = models.KnowledgeVectorIndexStatusActive
			exist.LastUsedAt = rec.LastUsedAt
			exist.LastError = ""
			if _, err := indexRepo.Update(ctx, exist); err != nil {
				return err
			}
			rec = exist
		}

		// 4) lock space to embedding profile + active index key
		space.EmbeddingProfileKey = strings.TrimSpace(in.EmbeddingProfileKey)
		space.ActiveVectorIndexKey = indexKey
		space.UpdatedBy = strings.TrimSpace(in.RequestedBy)
		if _, err := spaceRepo.Update(ctx, space); err != nil {
			return err
		}

		out.Space = space
		out.ActiveIndex = rec
		out.CreatedTable = createdTable
		return nil
	})
	if err != nil {
		return nil, err
	}

	// fresh load (avoid returning tx-bound pointers when tx rolled back)
	space, _ := spaces.FindByUUID(ctx, in.SpaceUUID)
	active, _ := indexes.FindBySpaceAndKey(ctx, in.SpaceUUID, out.ActiveIndex.IndexKey)
	out.Space = space
	out.ActiveIndex = active
	return &out, nil
}

func shortHash8(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}

// probeEmbeddingDimensions embeds a tiny sample to discover embedding dimension.
// It uses the tenant's current AI env and credentials from AI Settings store.
func (s *VectorIndexService) probeEmbeddingDimensions(ctx context.Context, tenantUUID, provider, model string) (dim int, env string, err error) {
	if s.agentSettings == nil {
		return 0, "", fmt.Errorf("agent settings unavailable")
	}
	env, configured, err := s.agentSettings.GetTenantCurrentAIEnv(ctx, tenantUUID)
	if err != nil && !isMissingTableError(err) {
		return 0, "", err
	}
	if !configured || strings.TrimSpace(env) == "" {
		env = "dev"
	}

	// If profile already probed, trust stored dimensions to avoid re-probe.
	if prof, e := s.agentSettings.GetProfile(ctx, env, &tenantUUID, "embedding", provider, model); e == nil && prof != nil {
		if agentSvc.EmbeddingProfileReady(prof) {
			dim := agentSvc.ResolveEmbeddingDimensions(prof)
			if dim > 0 {
				return dim, env, nil
			}
		}
	}

	// Reuse the exact vectorizer resolution logic used by ingestion, but force provider/model.
	prof, vec, err := (&IngestionService{agentSettings: s.agentSettings, vectorStore: &noopVectorStore{}}).resolveEmbeddingVectorizerForProfile(ctx, tenantUUID, env, provider, model)
	if err != nil {
		return 0, env, err
	}
	if vec == nil {
		return 0, env, fmt.Errorf("embedding_not_configured (provider=%s model=%s)", provider, model)
	}
	// If profile already has dimensions set, trust it.
	if prof != nil && prof.Dimensions > 0 {
		return prof.Dimensions, env, nil
	}
	out, err := vec.Embed(ctx, []string{"powerx-embedding-dim-probe"})
	if err != nil {
		return 0, env, err
	}
	if len(out) == 0 || len(out[0]) == 0 {
		return 0, env, fmt.Errorf("embedding probe returned empty vector")
	}
	return len(out[0]), env, nil
}

// noopVectorStore is a placeholder to satisfy IngestionService checks in resolveEmbeddingVectorizerForProfile.
type noopVectorStore struct{}

func (noopVectorStore) Driver() string { return "noop" }
func (noopVectorStore) Upsert(context.Context, uuid.UUID, []vectorstore.VectorRecord) error { return nil }
func (noopVectorStore) DeleteByChunkIDs(context.Context, uuid.UUID, []uuid.UUID) error { return nil }
func (noopVectorStore) DropSpace(context.Context, uuid.UUID) error { return nil }
func (noopVectorStore) Query(context.Context, vectorstore.QueryRequest) (vectorstore.QueryResponse, error) {
	return vectorstore.QueryResponse{}, nil
}
func (noopVectorStore) Health(context.Context) error { return nil }
func (noopVectorStore) Close(context.Context) error  { return nil }
