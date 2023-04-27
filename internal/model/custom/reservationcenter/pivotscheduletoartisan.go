package reservationcenter

import (
	"PowerX/internal/model/powermodel"
)

// Table Name
func (mdl *PivotScheduleToArtisan) TableName() string {
	return TableNamePivotScheduleToArtisan
}

type PivotScheduleToArtisan struct {
	powermodel.PowerPivot

	ScheduleId  int64 `gorm:"column:schedule_id; not null;index:idx_schedule_id; comment:形成Id" json:"scheduleId"`
	ArtisanId   int64 `gorm:"column:artisan_id; not null;index:idx_artisan_id; comment:ArtisanId" json:"artisanId"`
	IsAvailable bool  `gorm:"comment:该行程可以预约"  json:"isAvailable"`
}

const TableNamePivotScheduleToArtisan = "pivot_schedule_to_artisan"
