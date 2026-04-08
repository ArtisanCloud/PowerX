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
	ErrSkillInstallTaskNotFound = errors.New("skill install task not found")
)

type SkillInstallTaskFilter struct {
	Status   []string
	Provider string
	Repo     string
	SkillID  string
	Page     int
	PageSize int
	OrderBy  string
}

type SkillInstallTaskRepository struct {
	*baseRepo.BaseRepository[models.SkillInstallTask]
	db *gorm.DB
}

func NewSkillInstallTaskRepository(db *gorm.DB) *SkillInstallTaskRepository {
	if db == nil {
		panic("skill install task repository requires db")
	}
	return &SkillInstallTaskRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.SkillInstallTask](db),
		db:             db,
	}
}

func (r *SkillInstallTaskRepository) Create(ctx context.Context, task *models.SkillInstallTask) (*models.SkillInstallTask, error) {
	if task == nil {
		return nil, gorm.ErrInvalidData
	}
	task.Normalize()
	if err := r.db.WithContext(ctx).Create(task).Error; err != nil {
		// Backward compatibility: legacy table may miss tenant_uuid before migration runs.
		if isMissingTenantUUIDColumnErr(err) {
			if retryErr := r.db.WithContext(ctx).Omit("tenant_uuid").Create(task).Error; retryErr != nil {
				return nil, retryErr
			}
			return task, nil
		}
		return nil, err
	}
	return task, nil
}

func isMissingTenantUUIDColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, `column "tenant_uuid"`) && strings.Contains(msg, "skills_install_tasks") && strings.Contains(msg, "does not exist")
}

func (r *SkillInstallTaskRepository) GetByTaskID(ctx context.Context, taskID string) (*models.SkillInstallTask, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, gorm.ErrInvalidData
	}
	var task models.SkillInstallTask
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Take(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSkillInstallTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *SkillInstallTaskRepository) List(ctx context.Context, filter SkillInstallTaskFilter) ([]models.SkillInstallTask, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}

	query := r.db.WithContext(ctx).Model(&models.SkillInstallTask{})
	if filter.Provider != "" {
		query = query.Where("provider = ?", strings.ToLower(strings.TrimSpace(filter.Provider)))
	}
	if filter.Repo != "" {
		query = query.Where("repo = ?", strings.ToLower(strings.TrimSpace(filter.Repo)))
	}
	if filter.SkillID != "" {
		query = query.Where("skill_id = ?", strings.ToLower(strings.TrimSpace(filter.SkillID)))
	}
	if len(filter.Status) > 0 {
		query = query.Where("status IN ?", filter.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	orderBy := strings.TrimSpace(filter.OrderBy)
	if orderBy == "" {
		orderBy = "created_at DESC"
	}
	var rows []models.SkillInstallTask
	err := query.Order(orderBy).
		Offset((filter.Page - 1) * filter.PageSize).
		Limit(filter.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *SkillInstallTaskRepository) UpdateFields(ctx context.Context, taskID string, fields map[string]interface{}) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return gorm.ErrInvalidData
	}
	if len(fields) == 0 {
		return nil
	}
	result := r.db.WithContext(ctx).Model(&models.SkillInstallTask{}).Where("task_id = ?", taskID).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSkillInstallTaskNotFound
	}
	return nil
}
