package ops

import (
	"context"
	"fmt"
	"slices"
	"time"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type BackupArtifactRepository struct {
	*repository.BaseRepository[modelops.BackupArtifact]
	db *gorm.DB
}

func NewBackupArtifactRepository(db *gorm.DB) *BackupArtifactRepository {
	return &BackupArtifactRepository{
		BaseRepository: repository.NewBaseRepository[modelops.BackupArtifact](db),
		db:             db,
	}
}

func (r *BackupArtifactRepository) ListByJobID(ctx context.Context, jobID uint64) ([]modelops.BackupArtifact, error) {
	var rows []modelops.BackupArtifact
	err := r.db.WithContext(ctx).Where("job_id = ?", jobID).Order("id DESC").Find(&rows).Error
	return rows, err
}

func (r *BackupArtifactRepository) DeleteByJobID(ctx context.Context, jobID uint64) error {
	if jobID == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("job_id = ?", jobID).Delete(&modelops.BackupArtifact{}).Error
}

func (r *BackupArtifactRepository) GetLatestByJobID(ctx context.Context, jobID uint64) (*modelops.BackupArtifact, error) {
	if jobID == 0 {
		return nil, nil
	}
	var row modelops.BackupArtifact
	err := r.db.WithContext(ctx).
		Where("job_id = ?", jobID).
		Order("id DESC").
		Limit(1).
		Find(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, nil
	}
	return &row, nil
}

func (r *BackupArtifactRepository) GetLatestByJobIDs(ctx context.Context, jobIDs []uint64) (map[uint64]modelops.BackupArtifact, error) {
	jobIDs = slices.DeleteFunc(jobIDs, func(v uint64) bool { return v == 0 })
	if len(jobIDs) == 0 {
		return map[uint64]modelops.BackupArtifact{}, nil
	}
	var rows []modelops.BackupArtifact
	if err := r.db.WithContext(ctx).
		Where("job_id IN ?", jobIDs).
		Order("job_id ASC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[uint64]modelops.BackupArtifact, len(rows))
	for _, row := range rows {
		if _, exists := out[row.JobID]; exists {
			continue
		}
		out[row.JobID] = row
	}
	return out, nil
}

func (r *BackupArtifactRepository) DeleteByID(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&modelops.BackupArtifact{}).Error
}

// CleanupReviewRow 属于无 tenant_id 列的平台运维元数据，调用前由 Service 强制 Root。
// 左连接保留孤立／软删记录的可见性；一次查询避免每产物加载一次作业。
type CleanupReviewRow struct {
	ID              uint64
	JobID           uint64
	PolicyID        uint64
	CleanupState    string
	CleanupToken    string
	StorageURI      string
	SizeBytes       int64
	Checksum        string
	UpdatedAt       time.Time
	ArtifactDeleted bool
	JobFound        bool
	JobDeleted      bool
	JobStatus       string
	JobProtected    bool
}

func (r *BackupArtifactRepository) cleanupReviewQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Table(modelops.BackupArtifact{}.TableName()+" AS a").
		Joins("LEFT JOIN "+modelops.BackupJob{}.TableName()+" AS j ON j.id = a.job_id").
		Where("a.cleanup_state IN ?", []string{"pending", "unconfirmed"})
}
func cleanupReviewColumns() string {
	return "a.id, a.job_id, COALESCE(j.policy_id, 0) AS policy_id, a.cleanup_state, a.cleanup_token, a.storage_uri, a.size_bytes, a.checksum, a.updated_at, a.deleted_at IS NOT NULL AS artifact_deleted, j.id IS NOT NULL AS job_found, j.deleted_at IS NOT NULL AS job_deleted, COALESCE(j.status, '') AS job_status, COALESCE(j.protected, FALSE) AS job_protected"
}
func (r *BackupArtifactRepository) ListCleanupReviews(ctx context.Context, page, size int) ([]CleanupReviewRow, int64, error) {
	var total int64
	if err := r.cleanupReviewQuery(ctx).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []CleanupReviewRow{}
	err := r.cleanupReviewQuery(ctx).Select(cleanupReviewColumns()).Order("a.updated_at DESC, a.id DESC").Offset((page - 1) * size).Limit(size).Scan(&rows).Error
	return rows, total, err
}
func (r *BackupArtifactRepository) GetCleanupReview(ctx context.Context, id uint64) (CleanupReviewRow, error) {
	var row CleanupReviewRow
	err := r.cleanupReviewQuery(ctx).Select(cleanupReviewColumns()).Where("a.id = ?", id).Limit(1).Scan(&row).Error
	if err == nil && row.ID == 0 {
		err = gorm.ErrRecordNotFound
	}
	return row, err
}

// CleanupInventoryRows 批量读取当前元数据，保留缺失、孤立和软删观察；不限定当前清理状态。
func (r *BackupArtifactRepository) CleanupInventoryRows(ctx context.Context, tx *gorm.DB, ids []uint64) ([]CleanupReviewRow, error) {
	rows := []CleanupReviewRow{}
	if len(ids) == 0 {
		return rows, nil
	}
	if len(ids) > 4096 {
		return nil, fmt.Errorf("cleanup inventory identity limit")
	}
	e := tx.WithContext(ctx).Table(modelops.BackupArtifact{}.TableName()+" AS a").Joins("LEFT JOIN "+modelops.BackupJob{}.TableName()+" AS j ON j.id=a.job_id").Where("a.id IN ?", ids).Select(cleanupReviewColumns()).Scan(&rows).Error
	return rows, e
}

// QuarantinedInventoryKeys 检测本库中未能归属到当前 Scope 处置依据的标记，不赋予操作权限。
func (r *BackupArtifactRepository) QuarantinedInventoryKeys(ctx context.Context, tx *gorm.DB, limit int) ([]CleanupReviewRow, error) {
	rows := []CleanupReviewRow{}
	e := tx.WithContext(ctx).Table(modelops.BackupArtifact{}.TableName()+" AS a").Select("a.id,a.cleanup_token").Where("a.cleanup_state=?", "quarantined").Order("a.id ASC").Limit(limit).Scan(&rows).Error
	return rows, e
}
