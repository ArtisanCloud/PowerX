package knowledge

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// KnowledgeVectorIndexRepository 管理 space 的 dense 向量索引登记表。
type KnowledgeVectorIndexRepository struct {
	*baseRepo.BaseRepository[models.KnowledgeVectorIndex]
	db *gorm.DB
}

func NewKnowledgeVectorIndexRepository(db *gorm.DB) *KnowledgeVectorIndexRepository {
	if db == nil {
		panic("knowledge vector index repository requires db")
	}
	return &KnowledgeVectorIndexRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.KnowledgeVectorIndex](db),
		db:             db,
	}
}

func (r *KnowledgeVectorIndexRepository) FindBySpaceAndKey(ctx context.Context, spaceUUID uuid.UUID, indexKey string) (*models.KnowledgeVectorIndex, error) {
	indexKey = strings.TrimSpace(indexKey)
	if spaceUUID == uuid.Nil || indexKey == "" {
		return nil, gorm.ErrInvalidData
	}
	var out models.KnowledgeVectorIndex
	err := r.db.WithContext(ctx).
		Where("space_uuid = ? AND index_key = ?", spaceUUID, indexKey).
		Take(&out).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

func (r *KnowledgeVectorIndexRepository) FindActiveBySpace(ctx context.Context, spaceUUID uuid.UUID) (*models.KnowledgeVectorIndex, error) {
	if spaceUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var out models.KnowledgeVectorIndex
	err := r.db.WithContext(ctx).
		Where("space_uuid = ? AND status = ?", spaceUUID, models.KnowledgeVectorIndexStatusActive).
		Order("updated_at DESC, id DESC").
		Take(&out).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

func (r *KnowledgeVectorIndexRepository) ListBySpace(ctx context.Context, spaceUUID uuid.UUID, limit int) ([]models.KnowledgeVectorIndex, error) {
	if spaceUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var out []models.KnowledgeVectorIndex
	err := r.db.WithContext(ctx).
		Where("space_uuid = ?", spaceUUID).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&out).Error
	return out, err
}

func (r *KnowledgeVectorIndexRepository) TouchLastUsed(ctx context.Context, spaceUUID uuid.UUID, indexKey string, at time.Time) error {
	indexKey = strings.TrimSpace(indexKey)
	if spaceUUID == uuid.Nil || indexKey == "" {
		return gorm.ErrInvalidData
	}
	return r.db.WithContext(ctx).
		Model(&models.KnowledgeVectorIndex{}).
		Where("space_uuid = ? AND index_key = ?", spaceUUID, indexKey).
		Update("last_used_at", at).Error
}

