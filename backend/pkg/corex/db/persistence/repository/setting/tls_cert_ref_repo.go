// pkg/corex/db/persistence/repository/setting/tls_cert_ref_repo.go
package setting

import (
	"context"
	"time"

	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TLSCertRefRepository struct{ db *gorm.DB }

func NewTLSCertRefRepository(db *gorm.DB) *TLSCertRefRepository { return &TLSCertRefRepository{db: db} }

func (r *TLSCertRefRepository) with(ctx context.Context) *gorm.DB {
	db := r.db.WithContext(ctx)
	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		db = db.Debug()
	}
	return db
}

// 可通过 (tenant_id IS NOT DISTINCT FROM ?) 做 NULL 安全对比（系统级 vs 租户级）
func (r *TLSCertRefRepository) FindByKindRef(ctx context.Context, tenantID *uint64, kind, ref string) (*dbsetting.TLSCertRef, error) {
	var m dbsetting.TLSCertRef
	err := r.with(ctx).
		Where("(tenant_id IS NOT DISTINCT FROM ?) AND kind = ? AND ref = ?", tenantID, kind, ref).
		First(&m).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *TLSCertRefRepository) Create(ctx context.Context, m *dbsetting.TLSCertRef) error {
	return r.with(ctx).Create(m).Error
}

func (r *TLSCertRefRepository) Update(ctx context.Context, m *dbsetting.TLSCertRef) error {
	return r.with(ctx).Save(m).Error
}

func (r *TLSCertRefRepository) ListExpiringWithin(ctx context.Context, within time.Duration) ([]*dbsetting.TLSCertRef, error) {
	if within <= 0 {
		within = 30 * 24 * time.Hour
	}
	deadline := time.Now().Add(within)
	var list []*dbsetting.TLSCertRef
	err := r.with(ctx).
		Where("not_after IS NOT NULL AND not_after <= ?", deadline).
		Find(&list).Error
	return list, err
}

func (r *TLSCertRefRepository) Upsert(ctx context.Context, m *dbsetting.TLSCertRef) error {
	return r.with(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "kind"}, {Name: "ref"}},
		DoUpdates: clause.AssignmentColumns([]string{"subject", "fingerprint", "not_before", "not_after", "managed_by_acme", "updated_at"}),
	}).Create(m).Error
}
