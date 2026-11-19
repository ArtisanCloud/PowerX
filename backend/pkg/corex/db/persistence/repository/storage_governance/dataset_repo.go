package storagegovrepo

import (
	"context"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DatasetRepository owns admission snapshots for the platform-scoped directory.
type DatasetRepository struct {
	db           *gorm.DB
	deploymentID string
}

func NewDatasetRepository(db *gorm.DB, deploymentID string) *DatasetRepository {
	return &DatasetRepository{db: db, deploymentID: deploymentID}
}

// WithAdmission serializes registration per deployment. An atomic transaction
// protects the request idempotency key even across different datasets.
func (r *DatasetRepository) WithAdmission(ctx context.Context, fn func(*DatasetRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "storage-dataset:"+r.deploymentID).Error; err != nil {
			return err
		}
		return fn(NewDatasetRepository(tx, r.deploymentID))
	})
}

func (r *DatasetRepository) scope(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Unscoped().Where("tenant_id = ? AND deployment_id = ?", 0, r.deploymentID)
}

func (r *DatasetRepository) Request(ctx context.Context, request uuid.UUID) (m.DatasetRegistration, error) {
	var row m.DatasetRegistration
	err := r.scope(ctx).Where("request_uuid = ?", request).Take(&row).Error
	return row, err
}

func (r *DatasetRepository) Head(ctx context.Context, owner string, binding uuid.UUID, key string) (m.DatasetRegistration, error) {
	var row m.DatasetRegistration
	err := r.scope(ctx).Where("owner_id = ? AND binding_uuid = ? AND dataset_key = ? AND is_current", owner, binding, key).Take(&row).Error
	return row, err
}

func (r *DatasetRepository) Append(ctx context.Context, previous *m.DatasetRegistration, row *m.DatasetRegistration) error {
	if previous != nil {
		result := r.scope(ctx).Model(&m.DatasetRegistration{}).Where("id = ? AND is_current", previous.ID).Update("is_current", false)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
	}
	row.DeploymentID = r.deploymentID
	row.TenantID = 0
	return r.db.WithContext(ctx).Create(row).Error
}

// Heads includes retired admissions so historical ownership remains visible.
func (r *DatasetRepository) Heads(ctx context.Context, limit int) ([]m.DatasetRegistration, error) {
	var rows []m.DatasetRegistration
	err := r.scope(ctx).Where("is_current AND deleted_at IS NULL AND status = 1").Order("dataset_uuid").Limit(limit).Find(&rows).Error
	return rows, err
}
