package plugin_governance

import (
	"context"
	"errors"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_governance"
	corexrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

// ReportRepository persists governance reports.
type ReportRepository struct {
	corexrepo.BaseRepository[model.VersionGovernanceReport]
}

// NewReportRepository constructs the repository.
func NewReportRepository(db *gorm.DB) *ReportRepository {
	return &ReportRepository{BaseRepository: corexrepo.BaseRepository[model.VersionGovernanceReport]{DB: db}}
}

// Create inserts a report.
func (r *ReportRepository) Create(ctx context.Context, report *model.VersionGovernanceReport) (*model.VersionGovernanceReport, error) {
	if report == nil {
		return nil, errors.New("report is nil")
	}
	return report, r.DB.WithContext(ctx).Create(report).Error
}

// ListRecent returns latest reports ordered by time.
func (r *ReportRepository) ListRecent(ctx context.Context, tenantID string, limit int) ([]model.VersionGovernanceReport, error) {
	if limit <= 0 {
		limit = 20
	}
	query := r.DB.WithContext(ctx).Order("generated_at DESC")
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	var reports []model.VersionGovernanceReport
	if err := query.Limit(limit).Find(&reports).Error; err != nil {
		return nil, err
	}
	return reports, nil
}

// GetLatestByPlugin fetches the most recent report for tenant/plugin.
func (r *ReportRepository) GetLatestByPlugin(ctx context.Context, tenantID, pluginID string) (*model.VersionGovernanceReport, error) {
	var report model.VersionGovernanceReport
	err := r.DB.WithContext(ctx).
		Where("tenant_id = ? AND plugin_id = ?", tenantID, pluginID).
		Order("generated_at DESC").
		Take(&report).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &report, nil
}
