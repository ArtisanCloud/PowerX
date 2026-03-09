package skills

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

var (
	ErrSkillNotFound = errors.New("skill record not found")
)

// SkillRegistryFilter controls list query behavior.
type SkillRegistryFilter struct {
	SkillID   string
	Status    []string
	Source    []string
	Page      int
	PageSize  int
	OrderBy   string
	OnlyDraft bool
}

// SkillRegistryRepository persists skill registry records.
type SkillRegistryRepository struct {
	*baseRepo.BaseRepository[models.SkillRegistryRecord]
	db *gorm.DB
}

func NewSkillRegistryRepository(db *gorm.DB) *SkillRegistryRepository {
	if db == nil {
		panic("skill registry repository requires db")
	}
	return &SkillRegistryRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.SkillRegistryRecord](db),
		db:             db,
	}
}

func (r *SkillRegistryRepository) GetBySkillVersion(ctx context.Context, skillID, version string) (*models.SkillRegistryRecord, error) {
	skillID = strings.TrimSpace(strings.ToLower(skillID))
	version = strings.TrimSpace(version)
	if skillID == "" || version == "" {
		return nil, gorm.ErrInvalidData
	}
	var rec models.SkillRegistryRecord
	err := r.db.WithContext(ctx).Where("skill_id = ? AND version = ?", skillID, version).Take(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSkillNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *SkillRegistryRepository) GetLatestPublished(ctx context.Context, skillID string) (*models.SkillRegistryRecord, error) {
	skillID = strings.TrimSpace(strings.ToLower(skillID))
	if skillID == "" {
		return nil, gorm.ErrInvalidData
	}
	var rec models.SkillRegistryRecord
	err := r.db.WithContext(ctx).
		Where("skill_id = ? AND status = ? AND is_latest_published = ?", skillID, models.SkillStatusPublished, true).
		Order("updated_at DESC").
		Take(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSkillNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *SkillRegistryRepository) List(ctx context.Context, filter SkillRegistryFilter) ([]models.SkillRegistryRecord, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}

	query := r.db.WithContext(ctx).Model(&models.SkillRegistryRecord{})
	if filter.SkillID != "" {
		query = query.Where("skill_id = ?", strings.ToLower(strings.TrimSpace(filter.SkillID)))
	}
	if len(filter.Status) > 0 {
		query = query.Where("status IN ?", filter.Status)
	}
	if len(filter.Source) > 0 {
		query = query.Where("source IN ?", filter.Source)
	}
	if filter.OnlyDraft {
		query = query.Where("status = ?", models.SkillStatusDraft)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	orderBy := strings.TrimSpace(filter.OrderBy)
	if orderBy == "" {
		orderBy = "updated_at DESC"
	}

	var rows []models.SkillRegistryRecord
	err := query.Order(orderBy).
		Offset((filter.Page - 1) * filter.PageSize).
		Limit(filter.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// SetLatestPublished atomically switches latest published pointer to target version.
func (r *SkillRegistryRepository) SetLatestPublished(ctx context.Context, skillID, version, updatedBy, approvalNote string) error {
	skillID = strings.TrimSpace(strings.ToLower(skillID))
	version = strings.TrimSpace(version)
	if skillID == "" || version == "" {
		return gorm.ErrInvalidData
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.SkillRegistryRecord{}).
			Where("skill_id = ?", skillID).
			Updates(map[string]interface{}{
				"is_latest_published": false,
				"latest_switched_at":  time.Now(),
				"updated_by":          updatedBy,
			}).Error; err != nil {
			return err
		}

		result := tx.Model(&models.SkillRegistryRecord{}).
			Where("skill_id = ? AND version = ?", skillID, version).
			Updates(map[string]interface{}{
				"status":              models.SkillStatusPublished,
				"is_latest_published": true,
				"published_at":        time.Now(),
				"latest_switched_at":  time.Now(),
				"updated_by":          updatedBy,
				"approval_note":       approvalNote,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrSkillNotFound
		}
		return nil
	})
}

func (r *SkillRegistryRepository) UpsertDraft(ctx context.Context, rec *models.SkillRegistryRecord) (*models.SkillRegistryRecord, error) {
	if rec == nil {
		return nil, gorm.ErrInvalidData
	}
	rec.Normalize()
	rec.Status = models.SkillStatusDraft
	saved, err := r.BaseRepository.Upsert(ctx, rec, []clause.Column{{Name: "skill_id"}, {Name: "version"}})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

// SkillCapabilityBindingRepository handles capability bindings for skill versions.
type SkillCapabilityBindingRepository struct {
	*baseRepo.BaseRepository[models.SkillCapabilityBinding]
	db *gorm.DB
}

func NewSkillCapabilityBindingRepository(db *gorm.DB) *SkillCapabilityBindingRepository {
	if db == nil {
		panic("skill capability binding repository requires db")
	}
	return &SkillCapabilityBindingRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.SkillCapabilityBinding](db),
		db:             db,
	}
}

func (r *SkillCapabilityBindingRepository) ListBySkillVersion(ctx context.Context, skillID, version string) ([]models.SkillCapabilityBinding, error) {
	skillID = strings.TrimSpace(strings.ToLower(skillID))
	version = strings.TrimSpace(version)
	if skillID == "" || version == "" {
		return nil, gorm.ErrInvalidData
	}
	var rows []models.SkillCapabilityBinding
	err := r.db.WithContext(ctx).
		Where("skill_id = ? AND version = ?", skillID, version).
		Order("created_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
