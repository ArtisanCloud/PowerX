// pkg/corex/db/persistence/repository/audit/audit_event_repo.go
package audit

import (
	"context"
	"strings"
	"time"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type AuditEventRepository struct {
	*repository.BaseRepository[dbm.AuditEvent]
	db *gorm.DB
}

func NewAuditEventRepository(db *gorm.DB) *AuditEventRepository {
	return &AuditEventRepository{
		BaseRepository: repository.NewBaseRepository[dbm.AuditEvent](db),
		db:             db,
	}
}

/* ----------------------------------
   批量写入（最小可用）
----------------------------------- */

// InsertBatch 批量插入审计事件（已映射为持久化模型）
func (r *AuditEventRepository) InsertBatch(ctx context.Context, rows []dbm.AuditEvent) error {
	if len(rows) == 0 {
		return nil
	}

	return r.DB.WithContext(ctx).
		// A) 跳过模型 Hook（Before/AfterCreate 等）
		Session(&gorm.Session{SkipHooks: true}).
		// B) 关键：告诉回调“我在写审计表，别触发”
		Set("skip_audit", true).
		Create(&rows).Error
}

/* ----------------------------------
   映射工具：Service 层 → Model 层
----------------------------------- */

/* ----------------------------------
   单条查询 / 列表查询
----------------------------------- */

// FindByID 主键查询
func (r *AuditEventRepository) FindByID(ctx context.Context, id uint64) (*dbm.AuditEvent, error) {
	var row dbm.AuditEvent
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// 列表过滤器（按需增减）
type ListFilter struct {
	TenantUUID    string
	ResourceType  string
	ResourceID    string
	Operation     string
	Outcome       string
	ActorUserID   *int64
	CorrelationID string
	From, To      *time.Time // 时间范围（OccurredAt）

	Page    int
	Size    int
	OrderBy string // 默认 occurred_at DESC
}

// List 分页查询（常用筛选字段）
func (r *AuditEventRepository) List(ctx context.Context, f ListFilter) (list []dbm.AuditEvent, total int64, err error) {
	q := r.db.WithContext(ctx).Model(&dbm.AuditEvent{})

	if tenant := strings.TrimSpace(f.TenantUUID); tenant != "" {
		q = q.Where("tenant_uuid = ?", tenant)
	}
	if f.ResourceType != "" {
		q = q.Where("resource_type = ?", f.ResourceType)
	}
	if f.ResourceID != "" {
		q = q.Where("resource_id = ?", f.ResourceID)
	}
	if f.Operation != "" {
		q = q.Where("operation = ?", f.Operation)
	}
	if f.Outcome != "" {
		q = q.Where("outcome = ?", f.Outcome)
	}
	if f.ActorUserID != nil {
		q = q.Where("actor_user_id = ?", *f.ActorUserID)
	}
	if f.CorrelationID != "" {
		q = q.Where("correlation_id = ?", f.CorrelationID)
	}
	if f.From != nil {
		q = q.Where("occurred_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("occurred_at < ?", *f.To)
	}

	// 统计总数
	if err = q.Count(&total).Error; err != nil {
		return
	}

	// 分页
	if f.Page > 0 && f.Size > 0 {
		q = q.Offset((f.Page - 1) * f.Size).Limit(f.Size)
	}

	order := "occurred_at DESC"
	if f.OrderBy != "" {
		order = f.OrderBy
	}
	err = q.Order(order).Find(&list).Error
	return
}
