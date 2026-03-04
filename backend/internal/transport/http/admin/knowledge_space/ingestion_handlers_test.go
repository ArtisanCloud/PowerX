package knowledge_space

import (
	"context"
	"testing"
	"time"

	knowledgeModel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestActiveVectorDimensionsUsesActiveIndexRecord(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}

	if err := db.Exec(`ATTACH DATABASE ':memory:' AS public`).Error; err != nil {
		t.Fatalf("attach public schema failed: %v", err)
	}
	if err := db.Exec(`
CREATE TABLE public.knowledge_vector_indexes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  space_uuid TEXT,
  index_key TEXT,
  table_name TEXT,
  dimensions INTEGER,
  embedding_provider TEXT,
  embedding_model TEXT,
  embedding_profile_ref TEXT,
  status TEXT,
  last_used_at DATETIME,
  last_error TEXT,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME
)`).Error; err != nil {
		t.Fatalf("create table failed: %v", err)
	}

	spaceID := uuid.New()
	now := time.Now()
	rec := knowledgeModel.KnowledgeVectorIndex{
		SpaceUUID:         spaceID,
		IndexKey:          "idx-active",
		VectorTable:       "knowledge_vectors_1536",
		Dimensions:        1536,
		EmbeddingProvider: "openai",
		EmbeddingModel:    "text-embedding-3-small",
		Status:            knowledgeModel.KnowledgeVectorIndexStatusActive,
	}
	rec.CreatedAt = now
	rec.UpdatedAt = now
	if err := db.Table("public.knowledge_vector_indexes").Create(&rec).Error; err != nil {
		t.Fatalf("insert active index failed: %v", err)
	}

	h := &IngestionHandler{db: db}
	dim, err := h.activeVectorDimensions(context.Background(), spaceID)
	if err != nil {
		t.Fatalf("activeVectorDimensions returned error: %v", err)
	}
	if dim != 1536 {
		t.Fatalf("expected dim=1536, got %d", dim)
	}
}
