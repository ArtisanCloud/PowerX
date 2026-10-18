package media

import (
	"context"
	"errors"
	"reflect"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DerivativeState 只包含媒体对象和任务关联，不保存任务生命周期。
type DerivativeState struct {
	Asset    *m.MediaAsset
	Variants []*m.MediaAssetVariant
	Tx       *gorm.DB
}
type DerivativeRepository struct {
	*repository.BaseRepository[m.MediaAsset]
	db *gorm.DB
}

func NewDerivativeRepository(db *gorm.DB) *DerivativeRepository {
	return &DerivativeRepository{repository.NewBaseRepository[m.MediaAsset](db), db}
}
func (r *DerivativeRepository) WithAsset(ctx context.Context, tenant, asset string, apply func(*DerivativeState) error) error {
	return r.WithTx(ctx, nil, tenant, asset, apply)
}
func (r *DerivativeRepository) WithTx(ctx context.Context, tx *gorm.DB, tenant, asset string, apply func(*DerivativeState) error) error {
	if tenant == "" || asset == "" {
		return gorm.ErrRecordNotFound
	}
	work := func(tx *gorm.DB) error {
		var parent m.MediaAsset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND uuid = ?", tenant, asset).First(&parent).Error; err != nil {
			return err
		}
		before := append([]byte{}, parent.Meta...)
		var variants []*m.MediaAssetVariant
		if err := tx.Where("tenant_uuid = ? AND asset_uuid = ?", tenant, asset).Find(&variants).Error; err != nil {
			return err
		}
		old := map[string]m.MediaAssetVariant{}
		for _, v := range variants {
			old[v.Variant] = *v
		}
		state := &DerivativeState{Asset: &parent, Variants: variants, Tx: tx}
		if err := apply(state); err != nil {
			return err
		}
		if !reflect.DeepEqual(before, []byte(parent.Meta)) {
			if err := tx.Model(&parent).Where("tenant_uuid = ? AND uuid = ?", tenant, asset).Update("meta", parent.Meta).Error; err != nil {
				return err
			}
		}
		for _, v := range state.Variants {
			if prior, ok := old[v.Variant]; ok && reflect.DeepEqual(prior, *v) {
				continue
			}
			if v.TenantUUID != tenant || v.AssetUUID != asset {
				return errors.New("variant scope mismatch")
			}
			if err := tx.Save(v).Error; err != nil {
				return err
			}
		}
		return nil
	}
	if tx != nil {
		return work(tx.WithContext(ctx))
	}
	return r.db.WithContext(ctx).Transaction(work)
}
