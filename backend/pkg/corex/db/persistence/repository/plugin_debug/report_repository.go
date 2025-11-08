package plugin_debug

import (
	"context"
	"encoding/json"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_debug"
	corexrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ReportRepository persists diagnostic reports.
type ReportRepository struct {
	corexrepo.BaseRepository[model.DiagnosticReport]
}

// NewReportRepository constructs the repository.
func NewReportRepository(db *gorm.DB) *ReportRepository {
	return &ReportRepository{
		BaseRepository: corexrepo.BaseRepository[model.DiagnosticReport]{DB: db},
	}
}

// Create inserts a new report record.
func (r *ReportRepository) Create(ctx context.Context, report *model.DiagnosticReport) (*model.DiagnosticReport, error) {
	if report == nil {
		return nil, nil
	}
	err := r.DB.WithContext(ctx).Create(report).Error
	return report, err
}

// UpdateStatus sets the status field.
func (r *ReportRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, summary map[string]any, extras map[string]any) error {
	update := map[string]any{
		"status": status,
	}
	if summary != nil {
		if bytes, err := json.Marshal(summary); err == nil {
			update["summary"] = datatypes.JSON(bytes)
		} else {
			return err
		}
	}
	for k, v := range extras {
		update[k] = v
	}
	return r.DB.WithContext(ctx).
		Model(&model.DiagnosticReport{}).
		Where("uuid = ?", id).
		Updates(update).
		Error
}

// UpdateFields applies arbitrary field updates.
func (r *ReportRepository) UpdateFields(ctx context.Context, id uuid.UUID, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	return r.DB.WithContext(ctx).
		Model(&model.DiagnosticReport{}).
		Where("uuid = ?", id).
		Updates(values).
		Error
}

// Get fetches a report by UUID.
func (r *ReportRepository) Get(ctx context.Context, id uuid.UUID) (*model.DiagnosticReport, error) {
	var report model.DiagnosticReport
	if err := r.DB.WithContext(ctx).First(&report, "uuid = ?", id).Error; err != nil {
		return nil, err
	}
	return &report, nil
}
