package storagegovrepo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrControlInvalidInput = errors.New("invalid platform storage control identity")

var ErrControlIdempotencyConflict = errors.New("storage control request payload conflict")

const ControlExecutionDomain = "execution-control"

// ControlRepository 所有读写严格限定 tenant_id=0 和显式 ScopeID，不提供租户通配访问。
type ControlRepository struct {
	*repository.BaseRepository[m.ControlConfiguration]
}

func NewControlRepository(db *gorm.DB) *ControlRepository {
	return &ControlRepository{repository.NewBaseRepository[m.ControlConfiguration](db)}
}
func (r *ControlRepository) Ready(ctx context.Context) bool {
	return r.CheckReady(ctx) == nil
}

// CheckReady 保留缺结构与数据库不可读取的区别；探测不创建或修复资源。
func (r *ControlRepository) CheckReady(ctx context.Context) error {
	if r == nil || r.DB == nil {
		return &database.StorageGovernanceSchemaError{Unavailable: true, Reason: "database_unavailable"}
	}
	return database.CheckStorageGovernanceControlSchema(r.DB.WithContext(ctx))
}
func (r *ControlRepository) Get(ctx context.Context, scope, domain string) (*m.ControlConfiguration, error) {
	return r.GetTx(ctx, r.DB, scope, domain)
}
func (r *ControlRepository) GetTx(ctx context.Context, tx *gorm.DB, scope, domain string) (*m.ControlConfiguration, error) {
	var row m.ControlConfiguration
	err := tx.WithContext(ctx).Where("tenant_id = ? AND scope_id = ? AND domain = ?", 0, scope, domain).First(&row).Error
	return &row, err
}
func (r *ControlRepository) Revision(ctx context.Context, scope, domain string, revision int64) (*m.ControlRevision, error) {
	return r.RevisionTx(ctx, r.DB, scope, domain, revision)
}

// RevisionTx 在执行守卫的同一事务／连接内读取不可变版本。
func (r *ControlRepository) RevisionTx(ctx context.Context, tx *gorm.DB, scope, domain string, revision int64) (*m.ControlRevision, error) {
	var row m.ControlRevision
	e := tx.WithContext(ctx).Where("tenant_id = ? AND scope_id = ? AND domain = ? AND revision = ?", 0, scope, domain, revision).First(&row).Error
	return &row, e
}

func (r *ControlRepository) Request(ctx context.Context, scope, request string) (*m.ControlRevision, error) {
	return r.RequestTx(ctx, r.DB, scope, request)
}
func (r *ControlRepository) RequestTx(ctx context.Context, tx *gorm.DB, scope, request string) (*m.ControlRevision, error) {
	var row m.ControlRevision
	err := tx.WithContext(ctx).Where("tenant_id = ? AND scope_id = ? AND request_id = ?", 0, scope, request).First(&row).Error
	return &row, err
}

// Save 原子提交 CAS、不可变回执／审计和正式 TaskBus 的持久登记。
// register 仅可登记同库 TaskHistory，不得在事务中向 Redis 或外部服务发送消息。
func (r *ControlRepository) Save(ctx context.Context, row *m.ControlConfiguration, version *m.ControlRevision, expected int64, register func(context.Context, *gorm.DB) error) (*m.ControlRevision, bool, error) {
	return r.save(ctx, row, version, expected, false, register)
}

// SaveRecoveryTx 仅供受控恢复在安装独占锁内重建版本；不会降低本机水位，
// 仍以恢复后的当前版本作 CAS，配置、审计与任务登记随外层事务一起提交。
func (r *ControlRepository) SaveRecoveryTx(ctx context.Context, tx *gorm.DB, row *m.ControlConfiguration, version *m.ControlRevision, expected int64, register func(context.Context, *gorm.DB) error) (*m.ControlRevision, error) {
	if tx == nil {
		return nil, ErrControlInvalidInput
	}
	v, _, e := NewControlRepository(tx).save(ctx, row, version, expected, true, register)
	return v, e
}

func (r *ControlRepository) MaximumRevisionTx(ctx context.Context, tx *gorm.DB, scope, domain string) (int64, error) {
	var value int64
	e := tx.WithContext(ctx).Model(&m.ControlRevision{}).Where("tenant_id = ? AND scope_id = ? AND domain = ?", 0, scope, domain).Select("COALESCE(MAX(revision), 0)").Scan(&value).Error
	return value, e
}

func (r *ControlRepository) save(ctx context.Context, row *m.ControlConfiguration, version *m.ControlRevision, expected int64, recovery bool, register func(context.Context, *gorm.DB) error) (*m.ControlRevision, bool, error) {
	if row == nil || version == nil || row.TenantID != 0 || version.TenantID != 0 || row.ScopeID == "" || row.Domain == "" || row.HostID == "" || row.FilesystemID == "" || row.InstanceID == "" || version.RequestID == "" || version.RequestDigest == "" || version.TaskID == "" || !bytes.Equal(row.Document, version.Document) || version.ScopeID != row.ScopeID || version.Domain != row.Domain || expected < 0 || row.Revision <= expected || !recovery && row.Revision != expected+1 || version.Revision != row.Revision || version.Digest != row.Digest || version.ActorID != row.ActorID || row.ActorID == 0 {
		return nil, false, ErrControlInvalidInput
	}
	var result *m.ControlRevision
	replayed := false
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 固定顺序：执行开关 → 资源策略。恢复／交接核对不会与策略启用交错。
		if row.Domain != ControlExecutionDomain {
			if err := controlSharedLock(ctx, tx, row.ScopeID, ControlExecutionDomain); err != nil {
				return err
			}
		}
		if err := controlLock(ctx, tx, row.ScopeID, row.Domain); err != nil {
			return err
		}
		inserted := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(version)
		if inserted.Error != nil {
			return inserted.Error
		}
		if inserted.RowsAffected == 0 {
			var old m.ControlRevision
			err := tx.Where("tenant_id = ? AND scope_id = ? AND request_id = ?", 0, version.ScopeID, version.RequestID).First(&old).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRevisionConflict
			}
			if err != nil {
				return err
			}
			if old.RequestDigest != version.RequestDigest || old.Domain != version.Domain || old.ActorID != version.ActorID {
				return ErrControlIdempotencyConflict
			}
			result = &old
			replayed = true
			return nil
		}
		if expected == 0 {
			inserted = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
			if inserted.Error != nil {
				return inserted.Error
			}
			if inserted.RowsAffected != 1 {
				return ErrRevisionConflict
			}
		} else {
			updated := tx.Model(&m.ControlConfiguration{}).Where("tenant_id = ? AND scope_id = ? AND domain = ? AND revision = ?", 0, row.ScopeID, row.Domain, expected).
				Updates(map[string]any{"revision": row.Revision, "document": row.Document, "digest": row.Digest, "actor_id": row.ActorID, "publication_state": "pending", "problem": ""})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return ErrRevisionConflict
			}
		}
		if register == nil {
			return errors.New("storage control TaskBus registration unavailable")
		}
		if err := register(ctx, tx); err != nil {
			return err
		}
		result = version
		return nil
	})
	return result, replayed, err
}

// controlLock 覆盖首条配置尚不存在的接管窗口，与删除前的只读权威锁共用。
func controlLock(ctx context.Context, tx *gorm.DB, scope, domain string) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	sum := sha256.Sum256([]byte("powerx.storage.configuration/v1\x00" + scope + "\x00" + domain))
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(?)", int64(binary.BigEndian.Uint64(sum[:8]))).Error
}

func controlSharedLock(ctx context.Context, tx *gorm.DB, scope, domain string) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	sum := sha256.Sum256([]byte("powerx.storage.configuration/v1\x00" + scope + "\x00" + domain))
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock_shared(?)", int64(binary.BigEndian.Uint64(sum[:8]))).Error
}

// WithExecutionTx 对不存在的开关也取共享锁；只允许在同一连接内执行有界步骤。
func (r *ControlRepository) WithExecutionTx(ctx context.Context, tx *gorm.DB, scope string, fn func(*m.ControlConfiguration) error) error {
	if tx == nil {
		return ErrControlInvalidInput
	}
	if e := controlSharedLock(ctx, tx, scope, ControlExecutionDomain); e != nil {
		return e
	}
	var row m.ControlConfiguration
	e := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where("tenant_id = ? AND scope_id = ? AND domain = ?", 0, scope, ControlExecutionDomain).First(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return fn(nil)
	}
	if e != nil {
		return e
	}
	return fn(&row)
}

// ConfigurationsTx 只列出当前安装的平台配置；不接受租户或表名通配条件。
func (r *ControlRepository) ConfigurationsTx(ctx context.Context, tx *gorm.DB, scope string) ([]m.ControlConfiguration, error) {
	rows := []m.ControlConfiguration{}
	e := tx.WithContext(ctx).Where("tenant_id = ? AND scope_id = ?", 0, scope).Order("domain").Limit(129).Find(&rows).Error
	if e == nil && len(rows) > 128 {
		e = ErrControlInvalidInput
	}
	return rows, e
}

// WithCurrent 在配置写入使用的相同资源锁内执行有界操作；不存在的行也被隔离。
// 配置与预算账本按 DB → 主机账本取锁；不得持有发布所需的主机锁后反向阻塞等待 DB。
// 长任务的来源互斥租约必须为非阻塞锁，且配置发布不得取得该锁。
func (r *ControlRepository) WithCurrent(ctx context.Context, scope, domain string, fn func(*m.ControlConfiguration) error) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return r.WithCurrentTx(ctx, tx, scope, domain, fn) })
}

// WithCurrentTx 复用调用方事务，支持按固定顺序锁策略与目标；不额外借用连接。
func (r *ControlRepository) WithCurrentTx(ctx context.Context, tx *gorm.DB, scope, domain string, fn func(*m.ControlConfiguration) error) error {
	if tx == nil {
		return errors.New("storage control transaction required")
	}
	if e := controlLock(ctx, tx, scope, domain); e != nil {
		return e
	}
	var row m.ControlConfiguration
	e := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND scope_id = ? AND domain = ?", 0, scope, domain).First(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return fn(nil)
	}
	if e != nil {
		return e
	}
	return fn(&row)
}

// Apply 在正式 TaskBus 的业务事务内锁住当前权威，再执行有界本地发布并确认版本。
// apply 必须幂等：进程可能在文件发布后、数据库确认前退出。
func (r *ControlRepository) Apply(ctx context.Context, tx *gorm.DB, scope, domain string, revision int64, digest string, apply func(m.ControlConfiguration) error) (bool, error) {
	if tx == nil {
		return false, errors.New("storage control publication transaction required")
	}
	var current m.ControlConfiguration
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND scope_id = ? AND domain = ?", 0, scope, domain).First(&current).Error; err != nil {
		return false, err
	}
	if current.Revision != revision {
		return false, nil
	}
	if current.Digest != digest {
		return false, errors.New("storage control digest mismatch")
	}
	if current.AppliedRevision == revision && current.PublicationState == "applied" {
		return true, nil
	}
	if err := apply(current); err != nil {
		return false, err
	}
	updated := tx.WithContext(ctx).Model(&m.ControlConfiguration{}).Where("tenant_id = ? AND scope_id = ? AND domain = ? AND revision = ?", 0, scope, domain, revision).
		Updates(map[string]any{"applied_revision": revision, "publication_state": "applied", "problem": ""})
	if updated.Error != nil {
		return false, updated.Error
	}
	if updated.RowsAffected != 1 {
		return false, ErrRevisionConflict
	}
	return true, nil
}
func (r *ControlRepository) Fail(ctx context.Context, scope, domain string, revision int64, code string) error {
	return r.DB.WithContext(ctx).Model(&m.ControlConfiguration{}).Where("tenant_id = ? AND scope_id = ? AND domain = ? AND revision = ? AND applied_revision < ?", 0, scope, domain, revision, revision).
		Updates(map[string]any{"publication_state": "failed", "problem": code}).Error
}
