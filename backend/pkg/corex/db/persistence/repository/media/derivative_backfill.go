package media

import (
	"context"
	"errors"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"gorm.io/gorm"
)

// DerivativeCandidates uses an immutable ID high-water mark, never offset
// pagination. The caller must explicitly scope this maintenance operation.
func (r *AssetRepository) DerivativeCandidates(ctx context.Context, tenant string, assets []string, after, through uint64, limit int) ([]m.MediaAsset, uint64, error) {
	if tenant == "" {
		return nil, through, errors.New("tenant and batch size 1..200 required")
	}
	q := r.db.WithContext(ctx).Model(&m.MediaAsset{}).Where("tenant_uuid = ?", tenant)
	return derivativeCandidatePage(q, assets, after, through, limit)
}

// AllTenantDerivativeCandidates is exclusively for the trusted offline media
// maintenance command. Ordinary reads and every mutation remain tenant scoped.
func (r *AssetRepository) AllTenantDerivativeCandidates(ctx context.Context, assets []string, after, through uint64, limit int) ([]m.MediaAsset, uint64, error) {
	return derivativeCandidatePage(r.db.WithContext(ctx).Model(&m.MediaAsset{}), assets, after, through, limit)
}

func derivativeCandidatePage(q *gorm.DB, assets []string, after, through uint64, limit int) ([]m.MediaAsset, uint64, error) {
	if limit < 1 || limit > 200 {
		return nil, through, errors.New("batch size 1..200 required")
	}
	if len(assets) > 0 {
		q = q.Where("uuid IN ?", assets)
	}
	if through == 0 {
		var last m.MediaAsset
		err := q.Session(&gorm.Session{}).Order("id DESC").Limit(1).Find(&last).Error
		if err != nil {
			return nil, through, err
		}
		through = last.ID
	}
	var rows []m.MediaAsset
	err := q.Where("id > ? AND id <= ?", after, through).Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, through, err
}
