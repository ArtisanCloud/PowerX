package ops

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	base "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type BackupCleanupRetentionRepository struct {
	*base.BaseRepository[m.BackupCleanupRetention]
	db *gorm.DB
}

func NewBackupCleanupRetentionRepository(db *gorm.DB) *BackupCleanupRetentionRepository {
	return &BackupCleanupRetentionRepository{base.NewBaseRepository[m.BackupCleanupRetention](db), db}
}
func (r *BackupCleanupRetentionRepository) Ready(ctx context.Context) bool {
	if r.db == nil || r.db.Dialector.Name() != "postgres" {
		return false
	}
	// A read validates all required columns without loading records; request uniqueness is mandatory.
	var rows []m.BackupCleanupRetention
	if r.db.WithContext(ctx).Unscoped().Limit(0).Select("request_id,artifact_id,cleanup_token,actor_id,input_digest,review_version,scope_fingerprint,fence,policy_id,job_id,reason,state,problem,completed_at,created_at,updated_at,deleted_at").Find(&rows).Error != nil {
		return false
	}
	var ready bool
	e := r.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM pg_index i JOIN pg_class x ON x.oid=i.indexrelid WHERE i.indrelid=to_regclass(?) AND x.relname='uk_backup_retain_request' AND i.indisunique AND i.indisvalid AND i.indpred IS NULL AND i.indexprs IS NULL AND i.indkey::text=(SELECT attnum::text FROM pg_attribute WHERE attrelid=i.indrelid AND attname='request_id' AND NOT attisdropped))`, m.BackupCleanupRetention{}.TableName()).Scan(&ready).Error
	return e == nil && ready
}
func (r *BackupCleanupRetentionRepository) Request(ctx context.Context, id uuid.UUID) (m.BackupCleanupRetention, error) {
	var out m.BackupCleanupRetention
	e := r.db.WithContext(ctx).Unscoped().Where("request_id = ?", id).First(&out).Error
	return out, e
}
func (r *BackupCleanupRetentionRepository) Recent(ctx context.Context, id uint64, token, scope string) ([]m.BackupCleanupRetention, error) {
	out := []m.BackupCleanupRetention{}
	e := r.db.WithContext(ctx).Unscoped().Where("artifact_id = ? AND cleanup_token = ? AND scope_fingerprint = ?", id, token, scope).Order("id DESC").Limit(20).Find(&out).Error
	return out, e
}
func (r *BackupCleanupRetentionRepository) Admit(ctx context.Context, tx *gorm.DB, in *m.BackupCleanupRetention) (m.BackupCleanupRetention, error) {
	sum := sha256.Sum256([]byte(m.BackupCleanupRetention{}.TableName() + "/" + in.RequestID.String()))
	if e := tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(?)", int64(binary.BigEndian.Uint64(sum[:8]))).Error; e != nil {
		return m.BackupCleanupRetention{}, e
	}
	q := NewBackupCleanupRetentionRepository(tx)
	old, e := q.Request(ctx, in.RequestID)
	if e == nil {
		return old, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return old, e
	}
	var active m.BackupCleanupRetention
	e = tx.WithContext(ctx).Where("artifact_id = ? AND cleanup_token = ? AND state IN ?", in.ArtifactID, in.CleanupToken, []string{"checking", "retained"}).First(&active).Error
	if e == nil {
		now := time.Now().UTC()
		in.State = "rejected"
		in.Problem = "BACKUP_RETAIN_OTHER_REQUEST_ACTIVE"
		if active.State == "retained" {
			in.Problem = "BACKUP_RETAIN_ALREADY_RECORDED"
		}
		in.CompletedAt = &now
		e = tx.WithContext(ctx).Create(in).Error
		return *in, e
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return active, e
	}
	e = tx.WithContext(ctx).Create(in).Error
	return *in, e
}
func (r *BackupCleanupRetentionRepository) LockRequest(ctx context.Context, tx *gorm.DB, id uuid.UUID) (m.BackupCleanupRetention, error) {
	var out m.BackupCleanupRetention
	e := tx.WithContext(ctx).Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", id).First(&out).Error
	return out, e
}

// Finish is called in the same transaction as Protect. No terminal row can be rewritten.
func (r *BackupCleanupRetentionRepository) Finish(ctx context.Context, tx *gorm.DB, id uuid.UUID, state, problem string) error {
	now := time.Now().UTC()
	v := tx.WithContext(ctx).Model(&m.BackupCleanupRetention{}).Where("request_id = ? AND state = ?", id, "checking").Updates(map[string]any{"state": state, "problem": problem, "completed_at": now})
	if v.Error != nil {
		return v.Error
	}
	if v.RowsAffected != 1 {
		return errors.New("retention receipt changed")
	}
	return nil
}
func (r *BackupCleanupRetentionRepository) Protect(ctx context.Context, tx *gorm.DB, job uint64) error {
	v := tx.WithContext(ctx).Model(&m.BackupJob{}).Where("id = ? AND status = ?", job, m.BackupJobStatusSuccess).Update("protected", true)
	if v.Error != nil {
		return v.Error
	}
	if v.RowsAffected != 1 {
		return errors.New("retention job changed")
	}
	return nil
}
func (r *BackupCleanupRetentionRepository) HasActiveJobs(ctx context.Context, tx *gorm.DB, policy uint64) (bool, error) {
	var jobs int64
	e := tx.WithContext(ctx).Model(&m.BackupJob{}).Where("policy_id = ? AND status IN ?", policy, []string{"pending", "running"}).Count(&jobs).Error
	if e != nil || jobs > 0 {
		return jobs > 0, e
	}
	var drills int64
	e = tx.WithContext(ctx).Table(m.RestoreDrillRecord{}.TableName()+" AS d").Joins("JOIN "+m.BackupJob{}.TableName()+" AS j ON j.id = d.source_job_id").Where("j.policy_id = ? AND d.status IN ? AND d.deleted_at IS NULL", policy, []string{"queued", "running"}).Count(&drills).Error
	return drills > 0, e
}

// LockSources pins the exact metadata rows for the version check and protection write.
func (r *BackupCleanupRetentionRepository) LockSources(ctx context.Context, tx *gorm.DB, artifact, job uint64) error {
	var a m.BackupArtifact
	if e := tx.WithContext(ctx).Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, artifact).Error; e != nil {
		return e
	}
	var j m.BackupJob
	return tx.WithContext(ctx).Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&j, job).Error
}

// HistorySnapshot is platform-only audit, including soft-deleted corruption.
// The fixed bound makes a too-large source observable without importing a prefix.
func (r *BackupCleanupRetentionRepository) HistorySnapshot(ctx context.Context, tx *gorm.DB, scope string, lock bool, limit int) ([]m.BackupCleanupRetention, int64, int64, error) {
	if lock {
		if e := tx.WithContext(ctx).Exec("LOCK TABLE " + pgx.Identifier{coremodel.PowerXSchema, coremodel.TableOpsBackupCleanupRetentions}.Sanitize() + " IN SHARE MODE").Error; e != nil {
			return nil, 0, 0, e
		}
	}
	return r.HistorySnapshotSchema(ctx, tx, coremodel.PowerXSchema, scope, limit)
}
func (r *BackupCleanupRetentionRepository) HistorySnapshotSchema(ctx context.Context, db *gorm.DB, schema, scope string, limit int) ([]m.BackupCleanupRetention, int64, int64, error) {
	table := pgx.Identifier{schema, coremodel.TableOpsBackupCleanupRetentions}.Sanitize()
	q := db.WithContext(ctx).Unscoped().Table(table)
	var total, foreign int64
	if scope != "" {
		if e := q.Where("scope_fingerprint <> ?", scope).Count(&foreign).Error; e != nil {
			return nil, 0, 0, e
		}
		q = db.WithContext(ctx).Unscoped().Table(table).Where("scope_fingerprint = ?", scope)
	}
	if e := q.Count(&total).Error; e != nil {
		return nil, 0, 0, e
	}
	rows := []m.BackupCleanupRetention{}
	e := q.Order("request_id ASC, id ASC").Limit(limit).Find(&rows).Error
	return rows, total, foreign, e
}
