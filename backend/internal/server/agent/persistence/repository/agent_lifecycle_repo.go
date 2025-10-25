package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

var (
	// ErrAgentProfileNotFound 表示代理档案不存在。
	ErrAgentProfileNotFound = gorm.ErrRecordNotFound
	// ErrAgentAliasConflict 表示租户下代理别名冲突。
	ErrAgentAliasConflict = errors.New("agent alias already exists under tenant")
	// ErrLifecycleEventNotFound 表示生命周期事件不存在。
	ErrLifecycleEventNotFound = gorm.ErrRecordNotFound
)

// AgentProfileLifecycleRepository 负责生命周期档案。
type AgentProfileLifecycleRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentProfileLifecycle]
	db *gorm.DB
}

func NewAgentProfileLifecycleRepository(db *gorm.DB) *AgentProfileLifecycleRepository {
	return &AgentProfileLifecycleRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentProfileLifecycle](db),
		db:             db,
	}
}

func (r *AgentProfileLifecycleRepository) Create(ctx context.Context, profile *dbmodel.AgentProfileLifecycle) (*dbmodel.AgentProfileLifecycle, error) {
	if profile == nil {
		return nil, gorm.ErrInvalidData
	}
	if err := r.db.WithContext(ctx).Create(profile).Error; err != nil {
		if isUniqViolation(err) {
			return nil, ErrAgentAliasConflict
		}
		return nil, err
	}
	return profile, nil
}

func (r *AgentProfileLifecycleRepository) Save(ctx context.Context, profile *dbmodel.AgentProfileLifecycle) (*dbmodel.AgentProfileLifecycle, error) {
	if profile == nil {
		return nil, gorm.ErrInvalidData
	}
	if err := r.db.WithContext(ctx).
		Model(&dbmodel.AgentProfileLifecycle{}).
		Where("uuid = ?", profile.UUID).
		Select("*").
		Updates(profile).Error; err != nil {
		return nil, err
	}
	return profile, nil
}

func (r *AgentProfileLifecycleRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*dbmodel.AgentProfileLifecycle, error) {
	var out dbmodel.AgentProfileLifecycle
	if err := r.db.WithContext(ctx).Where("uuid = ?", id).First(&out).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentProfileNotFound
		}
		return nil, err
	}
	return &out, nil
}

func (r *AgentProfileLifecycleRepository) GetByTenantAlias(ctx context.Context, tenantID, alias string) (*dbmodel.AgentProfileLifecycle, error) {
	var out dbmodel.AgentProfileLifecycle
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND alias = ?", tenantID, alias).
		First(&out).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentProfileNotFound
		}
		return nil, err
	}
	return &out, nil
}

func (r *AgentProfileLifecycleRepository) ListByTenant(ctx context.Context, tenantID string, offset, limit int) ([]dbmodel.AgentProfileLifecycle, int64, error) {
	query := r.db.WithContext(ctx).
		Model(&dbmodel.AgentProfileLifecycle{}).
		Where("tenant_id = ?", tenantID).
		Order("alias ASC")

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var list []dbmodel.AgentProfileLifecycle
	if err := query.Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// AgentLifecycleEventRepository 管理生命周期事件。
type AgentLifecycleEventRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentLifecycleEventRecord]
	db *gorm.DB
}

func NewAgentLifecycleEventRepository(db *gorm.DB) *AgentLifecycleEventRepository {
	return &AgentLifecycleEventRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentLifecycleEventRecord](db),
		db:             db,
	}
}

func (r *AgentLifecycleEventRepository) Append(ctx context.Context, evt *dbmodel.AgentLifecycleEventRecord) (*dbmodel.AgentLifecycleEventRecord, error) {
	if evt == nil {
		return nil, gorm.ErrInvalidData
	}
	if evt.OccurredAt.IsZero() {
		evt.OccurredAt = time.Now().UTC()
	}
	if err := r.db.WithContext(ctx).Create(evt).Error; err != nil {
		return nil, err
	}
	return evt, nil
}

func (r *AgentLifecycleEventRepository) LatestByAgent(ctx context.Context, agentUUID uuid.UUID) (*dbmodel.AgentLifecycleEventRecord, error) {
	var out dbmodel.AgentLifecycleEventRecord
	if err := r.db.WithContext(ctx).
		Where("agent_uuid = ?", agentUUID).
		Order("occurred_at DESC").
		First(&out).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLifecycleEventNotFound
		}
		return nil, err
	}
	return &out, nil
}

func (r *AgentLifecycleEventRepository) ListByAgent(ctx context.Context, agentUUID uuid.UUID, limit int) ([]dbmodel.AgentLifecycleEventRecord, error) {
	query := r.db.WithContext(ctx).
		Where("agent_uuid = ?", agentUUID).
		Order("occurred_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var out []dbmodel.AgentLifecycleEventRecord
	if err := query.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *AgentLifecycleEventRepository) LockLatestForUpdate(ctx context.Context, agentUUID uuid.UUID) (*dbmodel.AgentLifecycleEventRecord, error) {
	var out dbmodel.AgentLifecycleEventRecord
	if err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("agent_uuid = ?", agentUUID).
		Order("occurred_at DESC").
		First(&out).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLifecycleEventNotFound
		}
		return nil, err
	}
	return &out, nil
}

// AgentHealthSnapshotRepository 管理健康快照。
type AgentHealthSnapshotRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentHealthSnapshotRecord]
	db *gorm.DB
}

func NewAgentHealthSnapshotRepository(db *gorm.DB) *AgentHealthSnapshotRepository {
	return &AgentHealthSnapshotRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentHealthSnapshotRecord](db),
		db:             db,
	}
}

func (r *AgentHealthSnapshotRepository) Upsert(ctx context.Context, snap *dbmodel.AgentHealthSnapshotRecord) (*dbmodel.AgentHealthSnapshotRecord, error) {
	if snap == nil {
		return nil, gorm.ErrInvalidData
	}
	if snap.WindowStartedAt.IsZero() {
		snap.WindowStartedAt = time.Now().UTC().Truncate(time.Minute)
	}
	unique := []clause.Column{{Name: "agent_uuid"}, {Name: "window_started_at"}}
	if _, err := r.BaseRepository.Upsert(ctx, snap, unique); err != nil {
		return nil, err
	}
	return snap, nil
}

func (r *AgentHealthSnapshotRepository) ListByAgent(ctx context.Context, agentUUID uuid.UUID, rangeHours, limit int) ([]dbmodel.AgentHealthSnapshotRecord, error) {
	query := r.db.WithContext(ctx).
		Where("agent_uuid = ?", agentUUID).
		Order("window_started_at DESC")
	if rangeHours > 0 {
		cutoff := time.Now().Add(-time.Duration(rangeHours) * time.Hour)
		query = query.Where("window_started_at >= ?", cutoff)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var out []dbmodel.AgentHealthSnapshotRecord
	if err := query.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func isUniqViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique")
}
