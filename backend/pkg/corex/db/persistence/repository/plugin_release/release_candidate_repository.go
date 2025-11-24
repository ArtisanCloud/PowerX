package plugin_release

import (
	"context"
	"fmt"
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

// ReleaseCandidateListFilter captures pagination and filter params.
type ReleaseCandidateListFilter struct {
	Page           int
	Size           int
	TenantID       string
	PluginID       string
	VersionPrefix  string
	ApprovalStatus string
	GateStatus     string
	CreatedBy      string
}

// ReleaseCandidateWithSummary enriches a candidate with rollout/distribution summary fields.
type ReleaseCandidateWithSummary struct {
	models.PluginReleaseCandidate `gorm:"embedded"`
	PlanStatus                    string `gorm:"column:plan_status"`
	OfflinePackageStatus          string `gorm:"column:offline_package_status"`
	OfflinePackageCount           int64  `gorm:"column:offline_package_count"`
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
	if r.db.Dialector.Name() == "sqlite" {
		return r.BaseRepository.Create(ctx, candidate)
	}
	_ = r.ensureUniqueIndex(ctx) // best-effort; ignore error to allow retry logic below
	orig := candidate
	unique := []clause.Column{
		{Name: "tenant_id"},
		{Name: "plugin_id"},
		{Name: "version"},
	}
	candidate, err := r.Upsert(ctx, candidate, unique)
	// If ON CONFLICT fails due to missing index, try to create index once more and retry.
	if err != nil && isMissingConflictConstraint(err) {
		if idxErr := r.ensureUniqueIndex(ctx); idxErr == nil {
			if retry, retryErr := r.Upsert(ctx, orig, unique); retryErr == nil {
				return retry, nil
			}
		}
		// Fallback：无唯一索引时退化为“查找→更新/插入”以避免 42P10。
		active := candidate
		if active == nil {
			active = orig
		}
		if active == nil {
			return nil, err
		}
		if existing, findErr := r.GetByTenantPluginVersion(ctx, active.TenantID, active.PluginID, active.Version); findErr == nil && existing != nil {
			copyCandidateFields(existing, active)
			if saveErr := r.db.WithContext(ctx).Save(existing).Error; saveErr == nil {
				return existing, nil
			}
		}
		return r.BaseRepository.Create(ctx, active)
	}
	return candidate, err
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

// DeleteByUUID soft-deletes a candidate by UUID and updates the audit fields.
func (r *ReleaseCandidateRepository) DeleteByUUID(ctx context.Context, candidateUUID uuid.UUID, actor string) error {
	if candidateUUID == uuid.Nil {
		return gorm.ErrInvalidData
	}
	updates := map[string]interface{}{
		"updated_by": strings.TrimSpace(actor),
	}
	var candidate models.PluginReleaseCandidate
	err := r.db.WithContext(ctx).Where("uuid = ?", candidateUUID).First(&candidate).Error
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Model(&candidate).Updates(updates).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Delete(&candidate).Error
}

// GetByUUID fetches a candidate by its UUID.
func (r *ReleaseCandidateRepository) GetByUUID(ctx context.Context, candidateUUID uuid.UUID) (*models.PluginReleaseCandidate, error) {
	if candidateUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var candidate models.PluginReleaseCandidate
	err := r.db.WithContext(ctx).
		Where("uuid = ?", candidateUUID).
		Take(&candidate).Error
	if err != nil {
		return nil, err
	}
	return &candidate, nil
}

// FindLatestByTenantPlugin returns the latest candidate ordered by creation time.
func (r *ReleaseCandidateRepository) FindLatestByTenantPlugin(ctx context.Context, tenantID, pluginID string) (*models.PluginReleaseCandidate, error) {
	var candidate models.PluginReleaseCandidate
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND plugin_id = ?", tenantID, pluginID).
		Order("created_at DESC").
		Take(&candidate).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &candidate, nil
}

// Save persists an existing candidate.
func (r *ReleaseCandidateRepository) Save(ctx context.Context, candidate *models.PluginReleaseCandidate) error {
	if candidate == nil {
		return gorm.ErrInvalidData
	}
	return r.db.WithContext(ctx).Save(candidate).Error
}

// UpdateFieldsByUUID updates selected columns for a candidate.
func (r *ReleaseCandidateRepository) UpdateFieldsByUUID(ctx context.Context, candidateUUID uuid.UUID, fields map[string]interface{}) error {
	if candidateUUID == uuid.Nil {
		return gorm.ErrInvalidData
	}
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&models.PluginReleaseCandidate{}).
		Where("uuid = ?", candidateUUID).
		Updates(fields).Error
}

// ensureUniqueIndex guarantees the unique index exists (needed for Postgres ON CONFLICT).
func (r *ReleaseCandidateRepository) ensureUniqueIndex(ctx context.Context) error {
	table := models.PluginReleaseCandidate{}.TableName()
	sql := fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS idx_plugin_release_candidate_tenant_plugin_version ON %s (tenant_id, plugin_id, version);`, table)
	return r.db.WithContext(ctx).Exec(sql).Error
}

func isMissingConflictConstraint(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no unique or exclusion constraint matching the ON CONFLICT specification")
}

func copyCandidateFields(dst, src *models.PluginReleaseCandidate) {
	dst.BuildArtifactURI = src.BuildArtifactURI
	dst.CommitHash = src.CommitHash
	dst.ReleaseNotes = src.ReleaseNotes
	dst.GateStatus = src.GateStatus
	dst.ApprovalStatus = src.ApprovalStatus
	dst.FailureReason = src.FailureReason
	dst.Labels = src.Labels
	dst.RollbackPlanID = src.RollbackPlanID
	dst.SubmittedAt = src.SubmittedAt
	dst.GatesCheckedAt = src.GatesCheckedAt
	dst.ApprovedAt = src.ApprovedAt
	dst.AuditRef = src.AuditRef
	dst.CreatedBy = src.CreatedBy
	dst.UpdatedBy = src.UpdatedBy
}

// List returns paginated candidates with rollout/distribution summary.
func (r *ReleaseCandidateRepository) List(ctx context.Context, filter ReleaseCandidateListFilter) ([]ReleaseCandidateWithSummary, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Size <= 0 {
		filter.Size = 20
	}
	table := models.PluginReleaseCandidate{}.TableName()
	planTable := models.ReleasePlan{}.TableName()
	pkgTable := models.OfflineDistributionPackage{}.TableName()

	query := r.db.WithContext(ctx).Model(&models.PluginReleaseCandidate{})
	if v := strings.TrimSpace(filter.TenantID); v != "" {
		query = query.Where("tenant_id = ?", v)
	}
	if v := strings.TrimSpace(filter.PluginID); v != "" {
		query = query.Where("plugin_id = ?", v)
	}
	if v := strings.TrimSpace(filter.VersionPrefix); v != "" {
		query = query.Where("version LIKE ?", v+"%")
	}
	if v := strings.TrimSpace(filter.ApprovalStatus); v != "" {
		query = query.Where("approval_status = ?", strings.ToLower(v))
	}
	if v := strings.TrimSpace(filter.GateStatus); v != "" {
		query = query.Where("gate_status = ?", strings.ToLower(v))
	}
	if v := strings.TrimSpace(filter.CreatedBy); v != "" {
		query = query.Where("created_by = ?", v)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	selectClause := fmt.Sprintf(`%s.*,
		COALESCE((SELECT status FROM %s WHERE release_candidate_id = %s.id ORDER BY created_at DESC LIMIT 1),'') AS plan_status,
		COALESCE((SELECT status FROM %s WHERE release_candidate_id = %s.id ORDER BY created_at DESC LIMIT 1),'') AS offline_package_status,
		(SELECT COUNT(1) FROM %s WHERE release_candidate_id = %s.id) AS offline_package_count`,
		table, planTable, table, pkgTable, table, pkgTable, table)

	var list []ReleaseCandidateWithSummary
	if err := query.
		Select(selectClause).
		Order("created_at DESC").
		Offset((filter.Page - 1) * filter.Size).
		Limit(filter.Size).
		Scan(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}
