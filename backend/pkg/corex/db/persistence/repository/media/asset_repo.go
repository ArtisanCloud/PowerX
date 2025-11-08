package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	mediamodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// AssetRepository 封装媒体资产的持久化操作，包含 CRUD、分页过滤与软删除清理。
type AssetRepository struct {
	*repository.BaseRepository[mediamodel.MediaAsset]
	db *gorm.DB
}

// NewAssetRepository 创建媒体资产仓储实例。
func NewAssetRepository(db *gorm.DB) *AssetRepository {
	return &AssetRepository{
		BaseRepository: repository.NewBaseRepository[mediamodel.MediaAsset](db),
		db:             db,
	}
}

// AssetListFilter 定义分页查询时可用的过滤条件。
type AssetListFilter struct {
	TenantID       uint64
	UUIDs          []string
	Drivers        []string
	OwnerType      string
	OwnerID        string
	BusinessStatus []string
	Keyword        string
	TagsAll        []string
	IncludeDeleted bool
	OnlyDeleted    bool
	Page           int
	PageSize       int
	OrderBy        string
	CreatedFrom    *time.Time
	CreatedTo      *time.Time
	UpdatedFrom    *time.Time
	UpdatedTo      *time.Time
}

// CleanupFilter 定义软删除清理时的筛选条件。
type CleanupFilter struct {
	TenantID uint64
	Drivers  []string
	Before   time.Time
	Limit    int
}

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 200
)

func normalizePage(page int) int {
	if page <= 0 {
		return defaultPage
	}
	return page
}

func normalizeSize(size int) int {
	if size <= 0 {
		return defaultPageSize
	}
	if size > maxPageSize {
		return maxPageSize
	}
	return size
}

// List 根据过滤条件分页查询媒体资产，同时返回总条数。
func (r *AssetRepository) List(ctx context.Context, filter AssetListFilter) (assets []mediamodel.MediaAsset, total int64, err error) {
	query := r.db.WithContext(ctx).Model(&mediamodel.MediaAsset{})

	if filter.IncludeDeleted || filter.OnlyDeleted {
		query = query.Unscoped()
	}
	if filter.OnlyDeleted {
		query = query.Where("deleted_at IS NOT NULL")
	}

	if filter.TenantID > 0 {
		query = query.Where("tenant_id = ?", filter.TenantID)
	}
	if len(filter.UUIDs) > 0 {
		query = query.Where("uuid IN ?", filter.UUIDs)
	}
	if len(filter.Drivers) > 0 {
		query = query.Where("driver IN ?", filter.Drivers)
	}
	if filter.OwnerType != "" {
		query = query.Where("owner_type = ?", strings.TrimSpace(filter.OwnerType))
	}
	if filter.OwnerID != "" {
		query = query.Where("owner_id = ?", strings.TrimSpace(filter.OwnerID))
	}
	if len(filter.BusinessStatus) > 0 {
		for _, status := range filter.BusinessStatus {
			if err := coremodel.ValidateMediaAssetStatus(status); err != nil {
				return nil, 0, err
			}
		}
		query = query.Where("business_status IN ?", filter.BusinessStatus)
	}
	if kw := strings.TrimSpace(filter.Keyword); kw != "" {
		pattern := "%" + strings.ToLower(kw) + "%"
		query = query.Where("LOWER(name) LIKE ?", pattern)
	}
	if len(filter.TagsAll) > 0 {
		tagJSON, marshalErr := json.Marshal(filter.TagsAll)
		if marshalErr != nil {
			return nil, 0, fmt.Errorf("marshal tags filter failed: %w", marshalErr)
		}
		query = query.Where("tags @> ?", datatypes.JSON(tagJSON))
	}
	if filter.CreatedFrom != nil {
		query = query.Where("created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		query = query.Where("created_at < ?", *filter.CreatedTo)
	}
	if filter.UpdatedFrom != nil {
		query = query.Where("updated_at >= ?", *filter.UpdatedFrom)
	}
	if filter.UpdatedTo != nil {
		query = query.Where("updated_at < ?", *filter.UpdatedTo)
	}

	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := normalizePage(filter.Page)
	size := normalizeSize(filter.PageSize)
	query = query.Offset((page - 1) * size).Limit(size)

	order := sanitizeOrder(strings.TrimSpace(filter.OrderBy))
	err = query.Order(order).Find(&assets).Error
	return
}

var allowedOrderFields = map[string]struct{}{
	"created_at": {},
	"updated_at": {},
	"id":         {},
}

func sanitizeOrder(input string) string {
	if input == "" {
		return "created_at DESC"
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "created_at DESC"
	}

	column := strings.ToLower(parts[0])
	if _, ok := allowedOrderFields[column]; !ok {
		return "created_at DESC"
	}

	direction := "DESC"
	if len(parts) > 1 {
		dir := strings.ToUpper(parts[1])
		if dir == "ASC" || dir == "DESC" {
			direction = dir
		}
	}

	return fmt.Sprintf("%s %s", column, direction)
}

// FindByUUID 通过租户与 UUID 查询资产，可选包含软删除记录。
func (r *AssetRepository) FindByUUID(ctx context.Context, tenantID uint64, uuid string, includeDeleted bool) (*mediamodel.MediaAsset, error) {
	if uuid == "" {
		return nil, errors.New("uuid 不能为空")
	}

	query := r.db.WithContext(ctx)
	if includeDeleted {
		query = query.Unscoped()
	}

	var asset mediamodel.MediaAsset
	err := query.Where("tenant_id = ? AND uuid = ?", tenantID, uuid).First(&asset).Error
	if err != nil {
		return nil, err
	}
	return &asset, nil
}

// FindByStorageKey 根据驱动与存储键定位资产。
func (r *AssetRepository) FindByStorageKey(ctx context.Context, tenantID uint64, driver, storageKey string) (*mediamodel.MediaAsset, error) {
	if driver == "" || storageKey == "" {
		return nil, errors.New("driver 与 storageKey 不能为空")
	}
	var asset mediamodel.MediaAsset
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND driver = ? AND storage_key = ?", tenantID, driver, storageKey).
		First(&asset).Error
	if err != nil {
		return nil, err
	}
	return &asset, nil
}

// CreateAsset 创建新的媒体资产。
func (r *AssetRepository) CreateAsset(ctx context.Context, asset *mediamodel.MediaAsset) (*mediamodel.MediaAsset, error) {
	if asset == nil {
		return nil, errors.New("asset 不能为空")
	}
	return r.BaseRepository.Create(ctx, asset)
}

// UpdateAsset 更新媒体资产。
func (r *AssetRepository) UpdateAsset(ctx context.Context, asset *mediamodel.MediaAsset) (*mediamodel.MediaAsset, error) {
	if asset == nil {
		return nil, errors.New("asset 不能为空")
	}
	return r.BaseRepository.Update(ctx, asset)
}

// SoftDeleteByUUID 标记资产为软删除，并可选记录删除人。
func (r *AssetRepository) SoftDeleteByUUID(ctx context.Context, tenantID uint64, uuid string, deletedBy *uint64) error {
	if uuid == "" {
		return errors.New("uuid 不能为空")
	}

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}

	if deletedBy != nil {
		if err := tx.Model(&mediamodel.MediaAsset{}).
			Where("tenant_id = ? AND uuid = ?", tenantID, uuid).
			Update("deleted_by", *deletedBy).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	result := tx.Where("tenant_id = ? AND uuid = ?", tenantID, uuid).
		Delete(&mediamodel.MediaAsset{})
	if result.Error != nil {
		tx.Rollback()
		return result.Error
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		return gorm.ErrRecordNotFound
	}

	return tx.Commit().Error
}

// RestoreByUUID 取消软删除。
func (r *AssetRepository) RestoreByUUID(ctx context.Context, tenantID uint64, uuid string) error {
	if uuid == "" {
		return errors.New("uuid 不能为空")
	}
	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&mediamodel.MediaAsset{}).
		Where("tenant_id = ? AND uuid = ?", tenantID, uuid).
		Updates(map[string]interface{}{
			"deleted_at": gorm.Expr("NULL"),
			"deleted_by": nil,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CleanupCandidates 查询超过指定时间仍处于软删除状态的资产，用于后续物理清理。
func (r *AssetRepository) CleanupCandidates(ctx context.Context, filter CleanupFilter) ([]mediamodel.MediaAsset, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	query := r.db.WithContext(ctx).
		Unscoped().
		Where("deleted_at IS NOT NULL")

	if !filter.Before.IsZero() {
		query = query.Where("deleted_at < ?", filter.Before)
	}
	if filter.TenantID > 0 {
		query = query.Where("tenant_id = ?", filter.TenantID)
	}
	if len(filter.Drivers) > 0 {
		query = query.Where("driver IN ?", filter.Drivers)
	}

	var assets []mediamodel.MediaAsset
	if err := query.Order("deleted_at ASC").Limit(limit).Find(&assets).Error; err != nil {
		return nil, err
	}
	return assets, nil
}
