package skills

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

var (
	ErrSkillTraceNotFound = errors.New("skill execution trace not found")
)

// SkillExecutionTraceFilter controls query conditions for trace list.
type SkillExecutionTraceFilter struct {
	TenantUUID string
	SkillID    string
	Version    string
	Status     []string
	Limit      int
	Offset     int
	OrderBy    string
}

// SkillExecutionTraceRepository persists execution traces.
type SkillExecutionTraceRepository struct {
	*baseRepo.BaseRepository[models.SkillExecutionTrace]
	db *gorm.DB
}

func NewSkillExecutionTraceRepository(db *gorm.DB) *SkillExecutionTraceRepository {
	if db == nil {
		panic("skill execution trace repository requires db")
	}
	return &SkillExecutionTraceRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.SkillExecutionTrace](db),
		db:             db,
	}
}

func (r *SkillExecutionTraceRepository) GetByTraceID(ctx context.Context, traceID string) (*models.SkillExecutionTrace, error) {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return nil, gorm.ErrInvalidData
	}
	var rec models.SkillExecutionTrace
	err := r.db.WithContext(ctx).Where("trace_id = ?", traceID).Take(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSkillTraceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *SkillExecutionTraceRepository) List(ctx context.Context, filter SkillExecutionTraceFilter) ([]models.SkillExecutionTrace, error) {
	query := r.db.WithContext(ctx).Model(&models.SkillExecutionTrace{})

	if filter.TenantUUID != "" {
		query = query.Where("tenant_uuid = ?", strings.ToLower(strings.TrimSpace(filter.TenantUUID)))
	}
	if filter.SkillID != "" {
		query = query.Where("skill_id = ?", strings.ToLower(strings.TrimSpace(filter.SkillID)))
	}
	if filter.Version != "" {
		query = query.Where("version = ?", strings.TrimSpace(filter.Version))
	}
	if len(filter.Status) > 0 {
		query = query.Where("status IN ?", filter.Status)
	}

	orderBy := strings.TrimSpace(filter.OrderBy)
	if orderBy == "" {
		orderBy = "created_at DESC"
	}
	query = query.Order(orderBy)

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var rows []models.SkillExecutionTrace
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// SkillLifecycleAuditRepository stores operation audit records.
type SkillLifecycleAuditRepository struct {
	*baseRepo.BaseRepository[models.SkillLifecycleAudit]
	db *gorm.DB
}

func NewSkillLifecycleAuditRepository(db *gorm.DB) *SkillLifecycleAuditRepository {
	if db == nil {
		panic("skill lifecycle audit repository requires db")
	}
	return &SkillLifecycleAuditRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.SkillLifecycleAudit](db),
		db:             db,
	}
}

func (r *SkillLifecycleAuditRepository) ListBySkill(ctx context.Context, skillID string, limit int) ([]models.SkillLifecycleAudit, error) {
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	if skillID == "" {
		return nil, gorm.ErrInvalidData
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	var rows []models.SkillLifecycleAudit
	err := r.db.WithContext(ctx).
		Where("skill_id = ?", skillID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
