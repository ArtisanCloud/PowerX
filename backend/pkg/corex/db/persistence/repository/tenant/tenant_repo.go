// pkg/corex/db/persistence/repository/tenant/tenant_repo.go
package tenant

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type TenantRepository struct {
	*repository.BaseRepository[dbm.Tenant]
	db *gorm.DB
}

func NewTenantRepository(db *gorm.DB) *TenantRepository {
	return &TenantRepository{
		BaseRepository: repository.NewBaseRepository[dbm.Tenant](db),
		db:             db,
	}
}

func (r *TenantRepository) GetByID(ctx context.Context, id uint64) (*dbm.Tenant, error) {
	var t dbm.Tenant
	if err := r.db.WithContext(ctx).First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TenantRepository) GetByKey(ctx context.Context, key string) (*dbm.Tenant, error) {
	var t dbm.Tenant
	if err := r.db.WithContext(ctx).Where("key = ?", key).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// EnsureByKey 并发安全地确保存在指定 key 的租户：
// - 存在：直接返回
// - 不存在：插入（ON CONFLICT DO NOTHING），随后再读一遍拿到 ID
func (r *TenantRepository) EnsureByKey(ctx context.Context, key, name, plan, tenantType string) (*dbm.Tenant, error) {
	// 先查
	if t, err := r.GetByKey(ctx, key); err == nil && t != nil {
		return t, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// 竞态安全地尝试创建
	toCreate := &dbm.Tenant{
		Key:    key,
		Name:   name,
		Plan:   plan,
		Type:   tenantType,
		Status: dbm.TenantStatusActive,
	}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}}, // 依赖 tenant.key 的唯一索引
			DoNothing: true,
		}).
		Create(toCreate).Error; err != nil {
		return nil, err
	}

	// 无论是我们创建还是别人抢先创建，这里再读一遍拿到最终记录（含 ID）
	return r.GetByKey(ctx, key)
}

// EnsureSystemTenant 语义化封装，复用 EnsureByKey
func (r *TenantRepository) EnsureSystemTenant(ctx context.Context) (*dbm.Tenant, error) {
	return r.EnsureByKey(ctx, dbm.SystemTenantKey, "System", dbm.TenantPlanFree, dbm.TenantTypeSystem)
}

// MapNamesByIDs 批量查询租户名称
func (r *TenantRepository) MapNamesByIDs(ctx context.Context, ids []uint64) (map[uint64]string, error) {
	mm := make(map[uint64]string, len(ids))
	if len(ids) == 0 {
		return mm, nil
	}
	var ts []dbm.Tenant
	if err := r.DB.WithContext(ctx).
		Table(model.TableIAMTenant).
		Where("id IN ?", ids).
		Find(&ts).Error; err != nil {
		return mm, err
	}
	for _, t := range ts {
		mm[t.ID] = t.Name
	}
	return mm, nil
}

// 追加：列表查询条件
type FindTenantsCond struct {
	Page, PageSize int
	SortBy         string
	SortOrder      string
	Keyword        string
	Status         *string
	Plan           *string
}

// 追加：列表 + 总数
func (r *TenantRepository) FindTenants(ctx context.Context, c FindTenantsCond) (items []dbm.Tenant, total int64, err error) {
	tx := r.DB.WithContext(ctx).Model(&dbm.Tenant{}).Where("deleted_at IS NULL")

	if c.Keyword != "" {
		like := "%" + c.Keyword + "%"
		// 如果是 MySQL, 改为 LIKE 并做 LOWER 兼容；Postgres 用 ILIKE
		tx = tx.Where("(name ILIKE ? OR domain ILIKE ?)", like, like)
	}
	if c.Status != nil && *c.Status != "" {
		tx = tx.Where("status = ?", *c.Status)
	}
	if c.Plan != nil && *c.Plan != "" {
		tx = tx.Where("plan = ?", *c.Plan)
	}

	if err = tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if c.Page <= 0 {
		c.Page = 1
	}
	if c.PageSize <= 0 || c.PageSize > 200 {
		c.PageSize = 20
	}
	if c.SortBy == "" {
		c.SortBy = "created_at"
	}
	if c.SortOrder != "asc" && c.SortOrder != "desc" {
		c.SortOrder = "desc"
	}

	if err = tx.Order(c.SortBy + " " + c.SortOrder).
		Offset((c.Page - 1) * c.PageSize).
		Limit(c.PageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return
}

// 追加：统计每个租户成员数量
func (r *TenantRepository) CountMembersByTenant(ctx context.Context) (map[uint64]int64, error) {
	type row struct {
		TenantID uint64
		Cnt      int64
	}
	var rows []row
	if err := r.DB.WithContext(ctx).
		Table(model.TableIAMMember).
		Select("tenant_id AS tenant_id, COUNT(1) AS cnt").
		Where("deleted_at IS NULL").
		Group("tenant_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[uint64]int64, len(rows))
	for _, r := range rows {
		out[r.TenantID] = r.Cnt
	}
	return out, nil
}
