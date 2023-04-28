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

	StoreId   int64 `gorm:"column:store_id; not null;index:idx_store_id" json:"storeId"`
	ArtisanId int64 `gorm:"column:artisan_id; not null;index:idx_artisan_id" json:"artisanId"`
}

const TableNamePivotStoreToArtisan = "pivot_store_to_artisan"
