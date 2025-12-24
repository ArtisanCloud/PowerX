package capability_registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// CapabilityRecordRepository 封装 CapabilityRecord 的 Postgres + Redis 访问。
type CapabilityRecordRepository struct {
	*baseRepo.BaseRepository[models.CapabilityRecord]

	db          *gorm.DB
	redis       redis.UniversalClient
	cachePrefix string
	cacheTTL    time.Duration
}

// CapabilityRecordRepositoryOptions 描述缓存行为。
type CapabilityRecordRepositoryOptions struct {
	CachePrefix string
	CacheTTL    time.Duration
}

const defaultRecordCacheTTL = 3 * time.Minute

// NewCapabilityRecordRepository 构造仓储实例。
func NewCapabilityRecordRepository(
	db *gorm.DB,
	redisClient redis.UniversalClient,
	opts ...CapabilityRecordRepositoryOptions,
) *CapabilityRecordRepository {
	if db == nil {
		panic("capability record repository requires DB")
	}

	var opt CapabilityRecordRepositoryOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.CachePrefix == "" {
		opt.CachePrefix = "capability_registry:cache"
	}
	if opt.CacheTTL <= 0 {
		opt.CacheTTL = defaultRecordCacheTTL
	}

	return &CapabilityRecordRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.CapabilityRecord](db),
		db:             db,
		redis:          redisClient,
		cachePrefix:    opt.CachePrefix,
		cacheTTL:       opt.CacheTTL,
	}
}

// CapabilityRecordFilter 支持按插件/状态筛选记录。
type CapabilityRecordFilter struct {
	CapabilityIDs []string
	PluginID      string
	Status        []string
	Limit         int
	Offset        int
	OrderBy       string
}

// Upsert 插入或更新能力记录（以 capability_id 作为唯一键）。
func (r *CapabilityRecordRepository) Upsert(ctx context.Context, record *models.CapabilityRecord) (*models.CapabilityRecord, error) {
	if record == nil {
		return nil, errors.New("capability record payload is nil")
	}
	saved, err := r.BaseRepository.Upsert(ctx, record, []clause.Column{{Name: "capability_id"}})
	if err != nil {
		return nil, err
	}
	if err := r.writeCache(ctx, saved); err != nil {
		logger.WarnF(ctx, "[capability_registry] cache capability failed id=%s err=%v", saved.CapabilityID, err)
	}
	return saved, nil
}

// GetByCapabilityID 通过 capability_id 查询记录，优先读取缓存。
func (r *CapabilityRecordRepository) GetByCapabilityID(ctx context.Context, capabilityID string) (*models.CapabilityRecord, error) {
	capabilityID = strings.TrimSpace(capabilityID)
	if capabilityID == "" {
		return nil, errors.New("capability_id is required")
	}

	if cached, ok, err := r.readCache(ctx, capabilityID); err != nil {
		logger.WarnF(ctx, "[capability_registry] read capability cache failed id=%s err=%v", capabilityID, err)
	} else if ok {
		return cached, nil
	}

	var record models.CapabilityRecord
	err := r.db.WithContext(ctx).
		Where("capability_id = ?", capabilityID).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCapabilityRecordNotFound
	}
	if err != nil {
		return nil, err
	}

	if err := r.writeCache(ctx, &record); err != nil {
		logger.WarnF(ctx, "[capability_registry] cache capability failed id=%s err=%v", capabilityID, err)
	}
	return &record, nil
}

// List 根据过滤条件列出能力记录。
func (r *CapabilityRecordRepository) List(ctx context.Context, filter CapabilityRecordFilter) ([]models.CapabilityRecord, error) {
	query := r.db.WithContext(ctx).Model(&models.CapabilityRecord{})

	if len(filter.CapabilityIDs) > 0 {
		query = query.Where("capability_id IN ?", filter.CapabilityIDs)
	}
	if filter.PluginID != "" {
		query = query.Where("plugin_id = ?", filter.PluginID)
	}
	if len(filter.Status) > 0 {
		query = query.Where("status IN ?", filter.Status)
	}

	orderBy := strings.TrimSpace(filter.OrderBy)
	if orderBy == "" {
		orderBy = "updated_at DESC"
	}
	query = query.Order(orderBy)

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var records []models.CapabilityRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// Count 返回满足过滤条件的能力记录数量。
func (r *CapabilityRecordRepository) Count(ctx context.Context, filter CapabilityRecordFilter) (int64, error) {
	query := r.db.WithContext(ctx).Model(&models.CapabilityRecord{})

	if len(filter.CapabilityIDs) > 0 {
		query = query.Where("capability_id IN ?", filter.CapabilityIDs)
	}
	if filter.PluginID != "" {
		query = query.Where("plugin_id = ?", filter.PluginID)
	}
	if len(filter.Status) > 0 {
		query = query.Where("status IN ?", filter.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// DeleteByCapabilityID 删除指定能力记录。
func (r *CapabilityRecordRepository) DeleteByCapabilityID(ctx context.Context, capabilityID string) error {
	capabilityID = strings.TrimSpace(capabilityID)
	if capabilityID == "" {
		return errors.New("capability_id is required")
	}
	if err := r.db.WithContext(ctx).
		Where("capability_id = ?", capabilityID).
		Delete(&models.CapabilityRecord{}).Error; err != nil {
		return err
	}
	r.invalidateCache(ctx, capabilityID)
	return nil
}

// InvalidateCache 主动失效缓存。
func (r *CapabilityRecordRepository) InvalidateCache(ctx context.Context, capabilityID string) {
	r.invalidateCache(ctx, capabilityID)
}

func (r *CapabilityRecordRepository) cacheEnabled() bool {
	if r.redis == nil || r.cachePrefix == "" {
		return false
	}
	// redis.UniversalClient 可能是 *redis.Client/*redis.ClusterClient 等指针类型，即使为 nil
	// 也会因为接口类型信息导致 r.redis != nil，需要额外检测。
	value := reflect.ValueOf(r.redis)
	return value.Kind() != reflect.Ptr || !value.IsNil()
}

func (r *CapabilityRecordRepository) cacheKey(capabilityID string) string {
	return fmt.Sprintf("%s:%s", r.cachePrefix, capabilityID)
}

func (r *CapabilityRecordRepository) writeCache(ctx context.Context, record *models.CapabilityRecord) error {
	if !r.cacheEnabled() || record == nil {
		return nil
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	ttl := r.cacheTTL
	if ttl <= 0 {
		ttl = defaultRecordCacheTTL
	}
	return r.redis.Set(ctx, r.cacheKey(record.CapabilityID), payload, ttl).Err()
}

func (r *CapabilityRecordRepository) readCache(ctx context.Context, capabilityID string) (*models.CapabilityRecord, bool, error) {
	if !r.cacheEnabled() || capabilityID == "" {
		return nil, false, nil
	}
	raw, err := r.redis.Get(ctx, r.cacheKey(capabilityID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var record models.CapabilityRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, false, err
	}
	return &record, true, nil
}

func (r *CapabilityRecordRepository) invalidateCache(ctx context.Context, capabilityID string) {
	if !r.cacheEnabled() || strings.TrimSpace(capabilityID) == "" {
		return
	}
	if err := r.redis.Del(ctx, r.cacheKey(capabilityID)).Err(); err != nil && !errors.Is(err, redis.Nil) {
		logger.WarnF(ctx, "[capability_registry] invalidate capability cache failed id=%s err=%v", capabilityID, err)
	}
}
