package ops

import (
	"context"
	"errors"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	base "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type BackupCleanupResolutionRepository struct {
	*base.BaseRepository[m.BackupCleanupResolution]
	db *gorm.DB
}

func NewBackupCleanupResolutionRepository(db *gorm.DB) *BackupCleanupResolutionRepository {
	return &BackupCleanupResolutionRepository{base.NewBaseRepository[m.BackupCleanupResolution](db), db}
}
func (r *BackupCleanupResolutionRepository) Ready(ctx context.Context) bool {
	if r.db == nil || r.db.Dialector.Name() != "postgres" {
		return false
	}
	var rows []m.BackupCleanupResolution
	if r.db.WithContext(ctx).Unscoped().Limit(0).Select("operation,expected_diagnosis,diagnostic_digest,id,uuid,request_id,artifact_id,job_id,policy_id,cleanup_token,actor_id,input_digest,review_version,history_digest,scope_fingerprint,generation,fence,reason,state,problem,completed_at,created_at,updated_at,deleted_at").Find(&rows).Error != nil {
		return false
	}
	var ready bool
	e := r.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM pg_index i JOIN pg_class x ON x.oid=i.indexrelid WHERE i.indrelid=to_regclass(?) AND x.relname='uk_backup_resolution_request' AND i.indisunique AND i.indisvalid AND i.indpred IS NULL AND i.indexprs IS NULL AND i.indkey::text=(SELECT attnum::text FROM pg_attribute WHERE attrelid=i.indrelid AND attname='request_id' AND NOT attisdropped))`, m.BackupCleanupResolution{}.TableName()).Scan(&ready).Error
	return e == nil && ready
}
func (r *BackupCleanupResolutionRepository) Request(ctx context.Context, id uuid.UUID) (m.BackupCleanupResolution, error) {
	var out m.BackupCleanupResolution
	e := r.db.WithContext(ctx).Unscoped().Where("request_id=?", id).First(&out).Error
	return out, e
}

// Requests 对已固定的有界 UUID 集合一次查询，避免列表逐请求访问 SQL。
func (r *BackupCleanupResolutionRepository) Requests(ctx context.Context, ids []uuid.UUID) ([]m.BackupCleanupResolution, error) {
	if len(ids) > 20 {
		return nil, errors.New("cleanup resolution request batch too large")
	}
	out := []m.BackupCleanupResolution{}
	if len(ids) == 0 {
		return out, nil
	}
	e := r.db.WithContext(ctx).Unscoped().Where("request_id IN ?", ids).Limit(20).Find(&out).Error
	return out, e
}
func (r *BackupCleanupResolutionRepository) LockRequest(ctx context.Context, tx *gorm.DB, id uuid.UUID) (m.BackupCleanupResolution, error) {
	var out m.BackupCleanupResolution
	e := tx.WithContext(ctx).Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id=?", id).First(&out).Error
	return out, e
}
func (r *BackupCleanupResolutionRepository) Create(ctx context.Context, tx *gorm.DB, row *m.BackupCleanupResolution) error {
	return tx.WithContext(ctx).Create(row).Error
}
func (r *BackupCleanupResolutionRepository) ForIntent(ctx context.Context, tx *gorm.DB, id uint64, token string, limit int) ([]m.BackupCleanupResolution, error) {
	out := []m.BackupCleanupResolution{}
	e := tx.WithContext(ctx).Unscoped().Where("artifact_id=? AND cleanup_token=?", id, token).Order("request_id ASC").Limit(limit).Find(&out).Error
	return out, e
}
func (r *BackupCleanupResolutionRepository) Finish(ctx context.Context, tx *gorm.DB, id uuid.UUID, state, problem string) error {
	v := tx.WithContext(ctx).Model(&m.BackupCleanupResolution{}).Where("request_id=? AND state=?", id, "checking").Updates(map[string]any{"state": state, "problem": problem, "completed_at": time.Now().UTC().Truncate(time.Microsecond)})
	if v.Error != nil {
		return v.Error
	}
	if v.RowsAffected != 1 {
		return errors.New("cleanup resolution receipt changed")
	}
	return nil
}

// RetainCurrentIntent 与保护写入／终态回执在同一事务，token 保持原值。
func (r *BackupCleanupResolutionRepository) RetainCurrentIntent(ctx context.Context, tx *gorm.DB, id uint64, token string, updated time.Time) error {
	return r.DisposeCurrentIntent(ctx, tx, id, token, updated, "retained")
}
func (r *BackupCleanupResolutionRepository) DisposeCurrentIntent(ctx context.Context, tx *gorm.DB, id uint64, token string, updated time.Time, state string) error {
	if state != "retained" && state != "quarantined" {
		return errors.New("unsupported cleanup disposition")
	}
	v := tx.WithContext(ctx).Model(&m.BackupArtifact{}).Where("id=? AND cleanup_token=? AND updated_at=? AND cleanup_state IN ?", id, token, updated, []string{"pending", "unconfirmed"}).Update("cleanup_state", state)
	if v.Error != nil {
		return v.Error
	}
	if v.RowsAffected != 1 {
		return errors.New("cleanup intent changed")
	}
	return nil
}

// QuarantineInventory 固定当前安装 Scope 的有界来源；平台运维表不使用 tenant_id。
func (r *BackupCleanupResolutionRepository) QuarantineInventory(ctx context.Context, tx *gorm.DB, scope string, limit int) ([]m.BackupCleanupResolution, error) {
	rows := []m.BackupCleanupResolution{}
	e := tx.WithContext(ctx).Unscoped().Where("scope_fingerprint=? AND operation=?", scope, "quarantine_cleanup").Order("request_id ASC").Limit(limit).Find(&rows).Error
	return rows, e
}
