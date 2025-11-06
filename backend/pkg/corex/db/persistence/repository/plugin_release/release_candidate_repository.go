package plugin_release

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// ReleaseCandidateRepository handles CRUD for plugin release candidate records.
type ReleaseCandidateRepository struct {
	*baseRepo.BaseRepository[models.PluginReleaseCandidate]
	db *gorm.DB
}

// NewReleaseCandidateRepository constructs the repository instance.
func NewReleaseCandidateRepository(db *gorm.DB) *ReleaseCandidateRepository {
	if db == nil {
		panic("release candidate repository requires non-nil db")
	}
	return &ReleaseCandidateRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.PluginReleaseCandidate](db),
		db:             db,
	}
}

// CreateCandidate inserts a new candidate ensuring tenant/plugin/version uniqueness.
func (r *ReleaseCandidateRepository) CreateCandidate(ctx context.Context, candidate *models.PluginReleaseCandidate) (*models.PluginReleaseCandidate, error) {
	if candidate == nil {
		return nil, gorm.ErrInvalidData
	}
	unique := []clause.Column{
		{Name: "tenant_id"},
		{Name: "plugin_id"},
		{Name: "version"},
	}
	return r.Upsert(ctx, candidate, unique)
}

// UpdateGateStatus updates the gating status and optional failure reason.
func (r *ReleaseCandidateRepository) UpdateGateStatus(ctx context.Context, candidateUUID uuid.UUID, status, reason string) error {
	if candidateUUID == uuid.Nil {
		return gorm.ErrInvalidData
	}
	update := map[string]interface{}{
		"gate_status": strings.ToLower(status),
	}
	if reason != "" {
		update["failure_reason"] = reason
	}
	if strings.ToLower(status) == strings.ToLower(models.PluginReleaseGateStatusPassed) {
		update["failure_reason"] = ""
	}
	if _, err := r.updateByUUID(ctx, candidateUUID, update); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return ErrDuplicateCandidate
		}
		return err
	}
	return nil
}

// UpdateApprovalStatus toggles approval state and clears failure reason when approved.
func (r *ReleaseCandidateRepository) UpdateApprovalStatus(ctx context.Context, candidateUUID uuid.UUID, status string) error {
	if candidateUUID == uuid.Nil {
		return gorm.ErrInvalidData
	}
	data := map[string]interface{}{
		"approval_status": strings.ToLower(status),
	}
	if strings.EqualFold(status, models.PluginReleaseApprovalApproved) {
		data["failure_reason"] = ""
	}
	if _, err := r.updateByUUID(ctx, candidateUUID, data); err != nil {
		return err
	}
	return nil
}

// GetByTenantPluginVersion returns a candidate keyed by tenant/plugin/version.
func (r *ReleaseCandidateRepository) GetByTenantPluginVersion(ctx context.Context, tenantID, pluginID, version string) (*models.PluginReleaseCandidate, error) {
	var candidate models.PluginReleaseCandidate
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND plugin_id = ? AND version = ?", tenantID, pluginID, version).
		Take(&candidate).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &candidate, nil
}

func (r *ReleaseCandidateRepository) updateByUUID(ctx context.Context, candidateUUID uuid.UUID, fields map[string]interface{}) (*models.PluginReleaseCandidate, error) {
	if candidateUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var candidate models.PluginReleaseCandidate
	err := r.db.WithContext(ctx).Where("uuid = ?", candidateUUID).Take(&candidate).Error
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return &candidate, nil
	}
	if err := r.db.WithContext(ctx).Model(&candidate).Updates(fields).Error; err != nil {
		return nil, err
	}
	return &candidate, nil
}
