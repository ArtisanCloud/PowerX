package plugin_release

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// ReleasePlanRepository manages release plans and associated canary batches.
type ReleasePlanRepository struct {
	plans  *baseRepo.BaseRepository[models.ReleasePlan]
	canary *baseRepo.BaseRepository[models.CanaryDeploymentRecord]
	db     *gorm.DB
}

// NewReleasePlanRepository builds a repository instance.
func NewReleasePlanRepository(db *gorm.DB) *ReleasePlanRepository {
	if db == nil {
		panic("release plan repository requires non-nil db")
	}
	return &ReleasePlanRepository{
		plans:  baseRepo.NewBaseRepository[models.ReleasePlan](db),
		canary: baseRepo.NewBaseRepository[models.CanaryDeploymentRecord](db),
		db:     db,
	}
}

// CreatePlan persists a release plan with optional canary batch records.
func (r *ReleasePlanRepository) CreatePlan(ctx context.Context, plan *models.ReleasePlan, canaries []*models.CanaryDeploymentRecord) (*models.ReleasePlan, error) {
	if plan == nil {
		return nil, gorm.ErrInvalidData
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(plan).Error; err != nil {
			return err
		}
		if len(canaries) == 0 {
			return nil
		}
		for _, batch := range canaries {
			batch.ReleasePlanID = plan.ID
		}
		return tx.Create(&canaries).Error
	})
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// ReplaceCanaryRecords rewrites canary batches for a plan in a single transaction.
func (r *ReleasePlanRepository) ReplaceCanaryRecords(ctx context.Context, planID uint64, records []*models.CanaryDeploymentRecord) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("release_plan_id = ?", planID).Delete(&models.CanaryDeploymentRecord{}).Error; err != nil {
			return err
		}
		if len(records) == 0 {
			return nil
		}
		for _, rec := range records {
			rec.ReleasePlanID = planID
		}
		return tx.Create(&records).Error
	})
}

// UpdatePlanStatus updates the rollout status and timestamps.
func (r *ReleasePlanRepository) UpdatePlanStatus(ctx context.Context, planID uint64, status string, windowStart, windowEnd *time.Time) error {
	if planID == 0 {
		return gorm.ErrInvalidData
	}
	data := map[string]interface{}{
		"status": status,
	}
	if windowStart != nil {
		data["window_start"] = *windowStart
	}
	if windowEnd != nil {
		data["window_end"] = *windowEnd
	}
	return r.db.WithContext(ctx).
		Model(&models.ReleasePlan{}).
		Where("id = ?", planID).
		Updates(data).Error
}

// GetPlanByID retrieves a plan with eager loaded canary batches.
func (r *ReleasePlanRepository) GetPlanByID(ctx context.Context, planID uint64) (*models.ReleasePlan, error) {
	if planID == 0 {
		return nil, gorm.ErrInvalidData
	}
	var plan models.ReleasePlan
	err := r.db.WithContext(ctx).
		Preload("CanaryRecords").
		Where("id = ?", planID).
		Take(&plan).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &plan, nil
}

// GetPlanByCandidate retrieves the plan for a given candidate id.
func (r *ReleasePlanRepository) GetPlanByCandidate(ctx context.Context, candidateID uint64) (*models.ReleasePlan, error) {
	if candidateID == 0 {
		return nil, gorm.ErrInvalidData
	}
	var plan models.ReleasePlan
	err := r.db.WithContext(ctx).
		Where("release_candidate_id = ?", candidateID).
		Order("created_at DESC").
		Take(&plan).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &plan, nil
}

// AppendCanarySnapshot upserts canary data.
func (r *ReleasePlanRepository) AppendCanarySnapshot(ctx context.Context, record *models.CanaryDeploymentRecord) (*models.CanaryDeploymentRecord, error) {
	if record == nil {
		return nil, gorm.ErrInvalidData
	}
	if record.ReleasePlanID == 0 {
		return nil, gorm.ErrInvalidData
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.CanaryDeploymentRecord
		err := tx.Where("release_plan_id = ? AND batch_name = ?", record.ReleasePlanID, record.BatchName).
			Take(&existing).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return tx.Create(record).Error
			}
			return err
		}
		record.ID = existing.ID
		return tx.Model(&existing).Updates(map[string]interface{}{
			"tenant_scope":       record.TenantScope,
			"metric_snapshot":    record.MetricSnapshot,
			"threshold_breached": record.ThresholdBreached,
			"action_taken":       record.ActionTaken,
			"completed_at":       record.CompletedAt,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return record, nil
}

// DeletePlan removes a plan and all related canary batches.
func (r *ReleasePlanRepository) DeletePlan(ctx context.Context, planID uint64) error {
	if planID == 0 {
		return gorm.ErrInvalidData
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("release_plan_id = ?", planID).Delete(&models.CanaryDeploymentRecord{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", planID).Delete(&models.ReleasePlan{}).Error
	})
}

// DeletePlansByCandidate removes all plans and batches for a release candidate.
func (r *ReleasePlanRepository) DeletePlansByCandidate(ctx context.Context, candidateUUID uuid.UUID) error {
	if candidateUUID == uuid.Nil {
		return gorm.ErrInvalidData
	}
	var planIDs []uint64
	if err := r.db.WithContext(ctx).
		Model(&models.ReleasePlan{}).
		Where("release_candidate_id = (SELECT id FROM plugin_release_candidates WHERE uuid = ?)", candidateUUID).
		Pluck("id", &planIDs).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(planIDs) > 0 {
			if err := tx.Where("release_plan_id IN ?", planIDs).Delete(&models.CanaryDeploymentRecord{}).Error; err != nil {
				return err
			}
		}
		return tx.Where("release_candidate_id = (SELECT id FROM plugin_release_candidates WHERE uuid = ?)", candidateUUID).
			Delete(&models.ReleasePlan{}).Error
	})
}
