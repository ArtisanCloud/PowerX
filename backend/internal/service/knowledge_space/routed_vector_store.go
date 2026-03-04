package knowledge_space

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	pgvectorcfg "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore/pgvector"
)

var (
	// ErrVectorIndexNotActivated indicates the space has no active dense vector index binding.
	// It is treated as a degraded-but-OK state for ingestion/retrieval (no vectors written / vector channel disabled).
	ErrVectorIndexNotActivated = errors.New("vector_index_not_activated")
	// ErrVectorIndexInvalid indicates the active index key exists but cannot be resolved to a usable table.
	ErrVectorIndexInvalid = errors.New("vector_index_invalid")
)

// RoutedVectorStore routes vector operations by space_uuid -> active_vector_index_key -> knowledge_vector_indexes.table_name.
// It keeps the upstream `vectorstore.Store` interface unchanged, so existing services can depend on it.
//
// Current implementation:
// - Supports driver=pgvector only (others fall back to the base store).
// - Caches per-table pgvector stores (each one owns a pgxpool).
type RoutedVectorStore struct {
	db *gorm.DB

	baseDriver string
	baseStore  vectorstore.Store

	pgBase pgvectorcfg.Config

	mu     sync.Mutex
	stores map[string]vectorstore.Store // key=tableName (no schema)
}

type RoutedVectorStoreOptions struct {
	DB         *gorm.DB
	BaseDriver string
	BaseStore  vectorstore.Store
	PGVector   pgvectorcfg.Config
}

func NewRoutedVectorStore(opts RoutedVectorStoreOptions) *RoutedVectorStore {
	if opts.DB == nil {
		panic("routed vector store requires db")
	}
	return &RoutedVectorStore{
		db:         opts.DB,
		baseDriver: opts.BaseDriver,
		baseStore:  opts.BaseStore,
		pgBase:     opts.PGVector.WithDefaults(),
		stores:     map[string]vectorstore.Store{},
	}
}

func (s *RoutedVectorStore) Driver() string {
	if s.baseDriver != "" {
		return s.baseDriver
	}
	if s.baseStore != nil {
		return s.baseStore.Driver()
	}
	return vectorstore.DriverPGVector
}

func (s *RoutedVectorStore) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range s.stores {
		_ = st.Close(ctx)
	}
	s.stores = map[string]vectorstore.Store{}
	if s.baseStore != nil {
		return s.baseStore.Close(ctx)
	}
	return nil
}

func (s *RoutedVectorStore) Health(ctx context.Context) error {
	if s.baseStore != nil {
		return s.baseStore.Health(ctx)
	}
	// Best-effort: validate cached stores.
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range s.stores {
		if err := st.Health(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *RoutedVectorStore) Upsert(ctx context.Context, space uuid.UUID, vectors []vectorstore.VectorRecord) error {
	if s == nil || len(vectors) == 0 {
		return nil
	}
	st, indexKey, err := s.resolveActiveStore(ctx, space)
	if err != nil {
		return err
	}
	if st == nil {
		return ErrVectorIndexNotActivated
	}
	_ = knowledge.NewKnowledgeVectorIndexRepository(s.db).TouchLastUsed(ctx, space, indexKey, time.Now())
	return st.Upsert(ctx, space, vectors)
}

func (s *RoutedVectorStore) DeleteByChunkIDs(ctx context.Context, space uuid.UUID, chunkIDs []uuid.UUID) error {
	if s == nil || len(chunkIDs) == 0 {
		return nil
	}
	records, err := knowledge.NewKnowledgeVectorIndexRepository(s.db).ListBySpace(ctx, space, 200)
	if err != nil {
		return err
	}
	// 没有登记表记录：视作未启用 dense 索引。
	if len(records) == 0 {
		return nil
	}
	var lastErr error
	for i := range records {
		st, err := s.storeForIndexRecord(records[i].VectorTable, records[i].Dimensions)
		if err != nil {
			lastErr = err
			continue
		}
		if st == nil {
			continue
		}
		if err := st.DeleteByChunkIDs(ctx, space, chunkIDs); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (s *RoutedVectorStore) DropSpace(ctx context.Context, space uuid.UUID) error {
	if s == nil {
		return nil
	}
	records, err := knowledge.NewKnowledgeVectorIndexRepository(s.db).ListBySpace(ctx, space, 200)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	var lastErr error
	for i := range records {
		st, err := s.storeForIndexRecord(records[i].VectorTable, records[i].Dimensions)
		if err != nil {
			lastErr = err
			continue
		}
		if st == nil {
			continue
		}
		if err := st.DropSpace(ctx, space); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (s *RoutedVectorStore) Query(ctx context.Context, req vectorstore.QueryRequest) (vectorstore.QueryResponse, error) {
	if s == nil {
		return vectorstore.QueryResponse{}, ErrVectorIndexNotActivated
	}
	st, indexKey, err := s.resolveActiveStore(ctx, req.SpaceID)
	if err != nil {
		return vectorstore.QueryResponse{}, err
	}
	if st == nil {
		return vectorstore.QueryResponse{}, ErrVectorIndexNotActivated
	}
	_ = knowledge.NewKnowledgeVectorIndexRepository(s.db).TouchLastUsed(ctx, req.SpaceID, indexKey, time.Now())
	return st.Query(ctx, req)
}

func (s *RoutedVectorStore) resolveActiveStore(ctx context.Context, space uuid.UUID) (vectorstore.Store, string, error) {
	if space == uuid.Nil {
		return nil, "", gorm.ErrInvalidData
	}
	// 非 pgvector：保持现状（全局单 store）。
	if s.Driver() != vectorstore.DriverPGVector {
		if s.baseStore == nil {
			return nil, "", ErrVectorIndexNotActivated
		}
		return s.baseStore, "", nil
	}

	var spaceRow struct {
		ActiveVectorIndexKey string `gorm:"column:active_vector_index_key"`
	}
	if err := s.db.WithContext(ctx).
		Table("knowledge_spaces").
		Select("active_vector_index_key").
		Where("uuid = ?", space).
		Take(&spaceRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrVectorIndexNotActivated
		}
		return nil, "", err
	}
	indexKey := strings.TrimSpace(spaceRow.ActiveVectorIndexKey)
	if indexKey == "" {
		return nil, "", ErrVectorIndexNotActivated
	}
	rec, err := knowledge.NewKnowledgeVectorIndexRepository(s.db).FindBySpaceAndKey(ctx, space, indexKey)
	if err != nil {
		return nil, "", err
	}
	if rec == nil {
		return nil, "", fmt.Errorf("%w: active_index_key=%s not found", ErrVectorIndexInvalid, indexKey)
	}
	if rec.Dimensions <= 0 || strings.TrimSpace(rec.VectorTable) == "" {
		return nil, "", fmt.Errorf("%w: active_index_key=%s missing table/dim", ErrVectorIndexInvalid, indexKey)
	}
	st, err := s.storeForIndexRecord(rec.VectorTable, rec.Dimensions)
	if err != nil {
		return nil, "", err
	}
	return st, indexKey, nil
}

func (s *RoutedVectorStore) storeForIndexRecord(table string, dim int) (vectorstore.Store, error) {
	table = strings.TrimSpace(table)
	if table == "" || dim <= 0 {
		return nil, gorm.ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.stores[table]; ok && st != nil {
		return st, nil
	}
	cfg := s.pgBase.WithDefaults()
	cfg.Table = table
	cfg.Dimensions = dim
	cfg.EnableMigrations = false

	st, err := vectorstore.Open(vectorstore.DriverPGVector, cfg)
	if err != nil {
		return nil, err
	}
	s.stores[table] = st
	return st, nil
}
