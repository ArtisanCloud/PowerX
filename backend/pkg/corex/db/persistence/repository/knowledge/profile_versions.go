package knowledge

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type IngestionProfileVersionRepository struct {
	*baseRepo.BaseRepository[models.IngestionProfileVersion]
	db *gorm.DB
}

func NewIngestionProfileVersionRepository(db *gorm.DB) *IngestionProfileVersionRepository {
	if db == nil {
		panic("ingestion profile version repository requires db")
	}
	return &IngestionProfileVersionRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.IngestionProfileVersion](db),
		db:             db,
	}
}

func (r *IngestionProfileVersionRepository) GetByUUID(ctx context.Context, profileUUID uuid.UUID) (*models.IngestionProfileVersion, error) {
	if profileUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var row models.IngestionProfileVersion
	err := r.db.WithContext(ctx).Where("uuid = ?", profileUUID).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *IngestionProfileVersionRepository) ListByKey(ctx context.Context, tenantUUID, profileKey, status string, limit int) ([]models.IngestionProfileVersion, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	profileKey = strings.TrimSpace(profileKey)
	status = strings.ToLower(strings.TrimSpace(status))
	if tenantUUID == "" || profileKey == "" {
		return nil, gorm.ErrInvalidData
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	q := r.db.WithContext(ctx).Model(&models.IngestionProfileVersion{}).
		Where("tenant_uuid = ? AND profile_key = ?", tenantUUID, profileKey).
		Order("version DESC").
		Limit(limit)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var rows []models.IngestionProfileVersion
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *IngestionProfileVersionRepository) FindLatestPublished(ctx context.Context, tenantUUID, profileKey string) (*models.IngestionProfileVersion, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	profileKey = strings.TrimSpace(profileKey)
	if tenantUUID == "" || profileKey == "" {
		return nil, gorm.ErrInvalidData
	}
	var row models.IngestionProfileVersion
	err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND profile_key = ? AND status = ?", tenantUUID, profileKey, models.ProfileStatusPublished).
		Order("version DESC").
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *IngestionProfileVersionRepository) CreateDraft(ctx context.Context, tenantUUID, profileKey, displayName string, config datatypes.JSON, createdBy string) (*models.IngestionProfileVersion, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	profileKey = strings.TrimSpace(profileKey)
	displayName = strings.TrimSpace(displayName)
	createdBy = strings.TrimSpace(createdBy)
	if tenantUUID == "" || profileKey == "" {
		return nil, gorm.ErrInvalidData
	}
	if len(config) == 0 {
		config = datatypes.JSON([]byte("{}"))
	}
	var created *models.IngestionProfileVersion
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxVersion int
		if err := tx.Model(&models.IngestionProfileVersion{}).
			Where("tenant_uuid = ? AND profile_key = ?", tenantUUID, profileKey).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		row := &models.IngestionProfileVersion{
			TenantUUID:   tenantUUID,
			ProfileKey:   profileKey,
			Version:      maxVersion + 1,
			Status:       models.ProfileStatusDraft,
			DisplayName:  displayName,
			Config:       config,
			RollbackFromID: 0,
			CreatedBy:      createdBy,
			PublishedAt:    nil,
			PublishedBy:    "",
		}
		row.UUID = uuid.New()
		row.CreatedAt = now
		row.UpdatedAt = now
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (r *IngestionProfileVersionRepository) CreateRollbackDraft(ctx context.Context, profileUUID uuid.UUID, createdBy string) (*models.IngestionProfileVersion, error) {
	createdBy = strings.TrimSpace(createdBy)
	if profileUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var created *models.IngestionProfileVersion
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var base models.IngestionProfileVersion
		if err := tx.Where("uuid = ?", profileUUID).Take(&base).Error; err != nil {
			return err
		}
		var maxVersion int
		if err := tx.Model(&models.IngestionProfileVersion{}).
			Where("tenant_uuid = ? AND profile_key = ?", base.TenantUUID, base.ProfileKey).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		row := &models.IngestionProfileVersion{
			TenantUUID:  base.TenantUUID,
			ProfileKey:  base.ProfileKey,
			Version:     maxVersion + 1,
			Status:      models.ProfileStatusDraft,
			DisplayName: base.DisplayName,
			Config:      base.Config,
			RollbackFromID: base.ID,
			CreatedBy:      createdBy,
		}
		row.UUID = uuid.New()
		row.CreatedAt = now
		row.UpdatedAt = now
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return created, nil
}

func (r *IngestionProfileVersionRepository) Publish(ctx context.Context, profileUUID uuid.UUID, publishedBy string) (*models.IngestionProfileVersion, error) {
	publishedBy = strings.TrimSpace(publishedBy)
	if profileUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var updated *models.IngestionProfileVersion
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row models.IngestionProfileVersion
		if err := tx.Where("uuid = ?", profileUUID).Take(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.IngestionProfileVersion{}).
			Where("tenant_uuid = ? AND profile_key = ? AND status = ?", row.TenantUUID, row.ProfileKey, models.ProfileStatusPublished).
			Update("status", models.ProfileStatusArchived).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		updates := map[string]any{
			"status":       models.ProfileStatusPublished,
			"published_at": now,
			"published_by": publishedBy,
		}
		if err := tx.Model(&models.IngestionProfileVersion{}).
			Where("id = ?", row.ID).
			Updates(updates).Error; err != nil {
			return err
		}
		row.Status = models.ProfileStatusPublished
		row.PublishedAt = &now
		row.PublishedBy = publishedBy
		updated = &row
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return updated, nil
}

type IndexProfileVersionRepository struct {
	*baseRepo.BaseRepository[models.IndexProfileVersion]
	db *gorm.DB
}

func NewIndexProfileVersionRepository(db *gorm.DB) *IndexProfileVersionRepository {
	if db == nil {
		panic("index profile version repository requires db")
	}
	return &IndexProfileVersionRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.IndexProfileVersion](db),
		db:             db,
	}
}

func (r *IndexProfileVersionRepository) GetByUUID(ctx context.Context, profileUUID uuid.UUID) (*models.IndexProfileVersion, error) {
	if profileUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var row models.IndexProfileVersion
	err := r.db.WithContext(ctx).Where("uuid = ?", profileUUID).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *IndexProfileVersionRepository) ListByKey(ctx context.Context, tenantUUID, profileKey, status string, limit int) ([]models.IndexProfileVersion, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	profileKey = strings.TrimSpace(profileKey)
	status = strings.ToLower(strings.TrimSpace(status))
	if tenantUUID == "" || profileKey == "" {
		return nil, gorm.ErrInvalidData
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	q := r.db.WithContext(ctx).Model(&models.IndexProfileVersion{}).
		Where("tenant_uuid = ? AND profile_key = ?", tenantUUID, profileKey).
		Order("version DESC").
		Limit(limit)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var rows []models.IndexProfileVersion
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *IndexProfileVersionRepository) FindLatestPublished(ctx context.Context, tenantUUID, profileKey string) (*models.IndexProfileVersion, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	profileKey = strings.TrimSpace(profileKey)
	if tenantUUID == "" || profileKey == "" {
		return nil, gorm.ErrInvalidData
	}
	var row models.IndexProfileVersion
	err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND profile_key = ? AND status = ?", tenantUUID, profileKey, models.ProfileStatusPublished).
		Order("version DESC").
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *IndexProfileVersionRepository) CreateDraft(ctx context.Context, tenantUUID, profileKey, displayName string, config datatypes.JSON, createdBy string) (*models.IndexProfileVersion, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	profileKey = strings.TrimSpace(profileKey)
	displayName = strings.TrimSpace(displayName)
	createdBy = strings.TrimSpace(createdBy)
	if tenantUUID == "" || profileKey == "" {
		return nil, gorm.ErrInvalidData
	}
	if len(config) == 0 {
		config = datatypes.JSON([]byte("{}"))
	}
	var created *models.IndexProfileVersion
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxVersion int
		if err := tx.Model(&models.IndexProfileVersion{}).
			Where("tenant_uuid = ? AND profile_key = ?", tenantUUID, profileKey).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		row := &models.IndexProfileVersion{
			TenantUUID:  tenantUUID,
			ProfileKey:  profileKey,
			Version:     maxVersion + 1,
			Status:      models.ProfileStatusDraft,
			DisplayName: displayName,
			Config:      config,
			RollbackFromID: 0,
			CreatedBy:   createdBy,
		}
		row.UUID = uuid.New()
		row.CreatedAt = now
		row.UpdatedAt = now
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (r *IndexProfileVersionRepository) CreateRollbackDraft(ctx context.Context, profileUUID uuid.UUID, createdBy string) (*models.IndexProfileVersion, error) {
	createdBy = strings.TrimSpace(createdBy)
	if profileUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var created *models.IndexProfileVersion
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var base models.IndexProfileVersion
		if err := tx.Where("uuid = ?", profileUUID).Take(&base).Error; err != nil {
			return err
		}
		var maxVersion int
		if err := tx.Model(&models.IndexProfileVersion{}).
			Where("tenant_uuid = ? AND profile_key = ?", base.TenantUUID, base.ProfileKey).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		row := &models.IndexProfileVersion{
			TenantUUID:  base.TenantUUID,
			ProfileKey:  base.ProfileKey,
			Version:     maxVersion + 1,
			Status:      models.ProfileStatusDraft,
			DisplayName: base.DisplayName,
			Config:      base.Config,
			RollbackFromID: base.ID,
			CreatedBy:      createdBy,
		}
		row.UUID = uuid.New()
		row.CreatedAt = now
		row.UpdatedAt = now
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return created, nil
}

func (r *IndexProfileVersionRepository) Publish(ctx context.Context, profileUUID uuid.UUID, publishedBy string) (*models.IndexProfileVersion, error) {
	publishedBy = strings.TrimSpace(publishedBy)
	if profileUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var updated *models.IndexProfileVersion
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row models.IndexProfileVersion
		if err := tx.Where("uuid = ?", profileUUID).Take(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.IndexProfileVersion{}).
			Where("tenant_uuid = ? AND profile_key = ? AND status = ?", row.TenantUUID, row.ProfileKey, models.ProfileStatusPublished).
			Update("status", models.ProfileStatusArchived).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		updates := map[string]any{
			"status":       models.ProfileStatusPublished,
			"published_at": now,
			"published_by": publishedBy,
		}
		if err := tx.Model(&models.IndexProfileVersion{}).
			Where("id = ?", row.ID).
			Updates(updates).Error; err != nil {
			return err
		}
		row.Status = models.ProfileStatusPublished
		row.PublishedAt = &now
		row.PublishedBy = publishedBy
		updated = &row
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return updated, nil
}

type RAGProfileVersionRepository struct {
	*baseRepo.BaseRepository[models.RAGProfileVersion]
	db *gorm.DB
}

func NewRAGProfileVersionRepository(db *gorm.DB) *RAGProfileVersionRepository {
	if db == nil {
		panic("rag profile version repository requires db")
	}
	return &RAGProfileVersionRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.RAGProfileVersion](db),
		db:             db,
	}
}

func (r *RAGProfileVersionRepository) GetByUUID(ctx context.Context, profileUUID uuid.UUID) (*models.RAGProfileVersion, error) {
	if profileUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var row models.RAGProfileVersion
	err := r.db.WithContext(ctx).Where("uuid = ?", profileUUID).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *RAGProfileVersionRepository) ListByKey(ctx context.Context, tenantUUID, profileKey, status string, limit int) ([]models.RAGProfileVersion, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	profileKey = strings.TrimSpace(profileKey)
	status = strings.ToLower(strings.TrimSpace(status))
	if tenantUUID == "" || profileKey == "" {
		return nil, gorm.ErrInvalidData
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	q := r.db.WithContext(ctx).Model(&models.RAGProfileVersion{}).
		Where("tenant_uuid = ? AND profile_key = ?", tenantUUID, profileKey).
		Order("version DESC").
		Limit(limit)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var rows []models.RAGProfileVersion
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *RAGProfileVersionRepository) FindLatestPublished(ctx context.Context, tenantUUID, profileKey string) (*models.RAGProfileVersion, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	profileKey = strings.TrimSpace(profileKey)
	if tenantUUID == "" || profileKey == "" {
		return nil, gorm.ErrInvalidData
	}
	var row models.RAGProfileVersion
	err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND profile_key = ? AND status = ?", tenantUUID, profileKey, models.ProfileStatusPublished).
		Order("version DESC").
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *RAGProfileVersionRepository) CreateDraft(ctx context.Context, tenantUUID, profileKey, displayName string, config datatypes.JSON, createdBy string) (*models.RAGProfileVersion, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	profileKey = strings.TrimSpace(profileKey)
	displayName = strings.TrimSpace(displayName)
	createdBy = strings.TrimSpace(createdBy)
	if tenantUUID == "" || profileKey == "" {
		return nil, gorm.ErrInvalidData
	}
	if len(config) == 0 {
		config = datatypes.JSON([]byte("{}"))
	}
	var created *models.RAGProfileVersion
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxVersion int
		if err := tx.Model(&models.RAGProfileVersion{}).
			Where("tenant_uuid = ? AND profile_key = ?", tenantUUID, profileKey).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		row := &models.RAGProfileVersion{
			TenantUUID:  tenantUUID,
			ProfileKey:  profileKey,
			Version:     maxVersion + 1,
			Status:      models.ProfileStatusDraft,
			DisplayName: displayName,
			Config:      config,
			RollbackFromID: 0,
			CreatedBy:   createdBy,
		}
		row.UUID = uuid.New()
		row.CreatedAt = now
		row.UpdatedAt = now
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (r *RAGProfileVersionRepository) CreateRollbackDraft(ctx context.Context, profileUUID uuid.UUID, createdBy string) (*models.RAGProfileVersion, error) {
	createdBy = strings.TrimSpace(createdBy)
	if profileUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var created *models.RAGProfileVersion
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var base models.RAGProfileVersion
		if err := tx.Where("uuid = ?", profileUUID).Take(&base).Error; err != nil {
			return err
		}
		var maxVersion int
		if err := tx.Model(&models.RAGProfileVersion{}).
			Where("tenant_uuid = ? AND profile_key = ?", base.TenantUUID, base.ProfileKey).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		row := &models.RAGProfileVersion{
			TenantUUID:  base.TenantUUID,
			ProfileKey:  base.ProfileKey,
			Version:     maxVersion + 1,
			Status:      models.ProfileStatusDraft,
			DisplayName: base.DisplayName,
			Config:      base.Config,
			RollbackFromID: base.ID,
			CreatedBy:      createdBy,
		}
		row.UUID = uuid.New()
		row.CreatedAt = now
		row.UpdatedAt = now
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return created, nil
}

func (r *RAGProfileVersionRepository) Publish(ctx context.Context, profileUUID uuid.UUID, publishedBy string) (*models.RAGProfileVersion, error) {
	publishedBy = strings.TrimSpace(publishedBy)
	if profileUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var updated *models.RAGProfileVersion
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row models.RAGProfileVersion
		if err := tx.Where("uuid = ?", profileUUID).Take(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.RAGProfileVersion{}).
			Where("tenant_uuid = ? AND profile_key = ? AND status = ?", row.TenantUUID, row.ProfileKey, models.ProfileStatusPublished).
			Update("status", models.ProfileStatusArchived).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		updates := map[string]any{
			"status":       models.ProfileStatusPublished,
			"published_at": now,
			"published_by": publishedBy,
		}
		if err := tx.Model(&models.RAGProfileVersion{}).
			Where("id = ?", row.ID).
			Updates(updates).Error; err != nil {
			return err
		}
		row.Status = models.ProfileStatusPublished
		row.PublishedAt = &now
		row.PublishedBy = publishedBy
		updated = &row
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return updated, nil
}
