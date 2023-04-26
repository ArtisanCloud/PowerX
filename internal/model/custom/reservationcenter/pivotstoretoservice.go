package reservationcenter

import (
	"PowerX/internal/model/powermodel"
)

// Table Name
func (mdl *PivotStoreToService) TableName() string {
	return TableNamePivotStoreToService
}

type PivotStoreToService struct {
	powermodel.PowerPivot

	StoreId   int64 `gorm:"column:store_id; not null;index:idx_store_id; comment:店铺Id" json:"storeId"`
	ServiceId int64 `gorm:"column:service_id; not null;index:idx_service_id; comment:服务Id" json:"serviceId"`
}

const TableNamePivotStoreToService = "pivot_store_to_service"
