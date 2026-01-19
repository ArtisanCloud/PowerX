package knowledge

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
)

type KnowledgeChunkRepository struct {
	db *gorm.DB
}

func NewKnowledgeChunkRepository(db *gorm.DB) *KnowledgeChunkRepository {
	if db == nil {
		panic("knowledge chunk repository requires db")
	}
	return &KnowledgeChunkRepository{db: db}
}

func (r *KnowledgeChunkRepository) UpsertMany(ctx context.Context, rows []models.KnowledgeChunk) error {
	if r == nil || r.db == nil {
		return errors.New("db is nil")
	}
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "space_uuid"}, {Name: "chunk_uuid"}},
			DoUpdates: clause.AssignmentColumns([]string{"job_uuid", "kind", "content", "metadata", "updated_at"}),
		}).
		Create(&rows).Error
}

func (r *KnowledgeChunkRepository) ListByJob(ctx context.Context, spaceID uuid.UUID, jobID uuid.UUID, page int, pageSize int) ([]models.KnowledgeChunk, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("db is nil")
	}
	if spaceID == uuid.Nil || jobID == uuid.Nil {
		return nil, 0, gorm.ErrInvalidData
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	q := r.db.WithContext(ctx).Model(&models.KnowledgeChunk{}).
		Where("space_uuid = ? AND job_uuid = ?", spaceID, jobID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var out []models.KnowledgeChunk
	// Order by ingestion/segment order for stable preview:
	// 1) segment_part (1-based within segmenting mode)
	// 2) chunk_idx (global monotonic counter)
	// 3) created_at/chunk_uuid fallback
	//
	// NOTE: metadata is JSON; expression differs by dialect (sqlite/mysql/postgres).
	order := "updated_at ASC, chunk_uuid ASC"
	switch strings.ToLower(r.db.Dialector.Name()) {
	case "postgres":
		order = "COALESCE((metadata->>'segment_part')::int, 2147483647) ASC, COALESCE((metadata->>'chunk_idx')::int, 2147483647) ASC, created_at ASC, chunk_uuid ASC"
	case "sqlite":
		order = "COALESCE(CAST(json_extract(metadata, '$.segment_part') AS INTEGER), 2147483647) ASC, COALESCE(CAST(json_extract(metadata, '$.chunk_idx') AS INTEGER), 2147483647) ASC, created_at ASC, chunk_uuid ASC"
	case "mysql":
		order = "COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.segment_part')) AS SIGNED), 2147483647) ASC, COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.chunk_idx')) AS SIGNED), 2147483647) ASC, created_at ASC, chunk_uuid ASC"
	}
	err := q.Order(order).
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&out).Error
	return out, total, err
}

func (r *KnowledgeChunkRepository) FindOne(ctx context.Context, spaceID uuid.UUID, chunkID uuid.UUID) (*models.KnowledgeChunk, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("db is nil")
	}
	if spaceID == uuid.Nil || chunkID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var row models.KnowledgeChunk
	err := r.db.WithContext(ctx).
		Where("space_uuid = ? AND chunk_uuid = ?", spaceID, chunkID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func normalizeText(s string) string { return strings.TrimSpace(s) }
