package product

import (
	"PowerX/internal/model/powermodel"
)

// Table Name
func (mdl *PivotStoreToArtisan) TableName() string {
	return TableNamePivotStoreToArtisan
}

// 数据表结构
type PivotStoreToArtisan struct {
	powermodel.PowerPivot

	StoreID   int64 `gorm:"column:store_id; not null;index:idx_store_id" json:"storeID"`
	ArtisanID int64 `gorm:"column:artisan_id; not null;index:idx_artisan_id" json:"artisanID"`
}

const TableNamePivotStoreToArtisan = "pivot_store_to_artisan"
