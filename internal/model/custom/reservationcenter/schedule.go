package reservationcenter

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/product"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"time"
)

type Schedule struct {
	powermodel.PowerModel

	Store                  *product.Store            `gorm:"foreignKey:StoreId;references:Id" json:"store"`
	Reservations           []*Reservation            `gorm:"foreignKey:ScheduleId;references:Id" json:"reservations"`
	PivotScheduleToArtisan []*PivotScheduleToArtisan `gorm:"foreignKey:ScheduleId;references:Id" json:"pivotScheduleToArtisan"`

	StoreId            int64     `gorm:"comment:店铺Id" json:"storeId"`
	ApprovalStatus     string    `gorm:"comment:审批状态" json:"approvalStatus"`
	Capacity           int32     `gorm:"comment:最大客服服务容量" json:"capacity"`
	CopyFromScheduleId int64     `gorm:"comment:复制从日程表Id" json:"copyFromScheduleId"`
	Name               string    `gorm:"comment:名字" json:"name"`
	Description        string    `gorm:"comment:描述" json:"description"`
	IsActive           bool      `gorm:"comment:开放状态" json:"isActive"`
	Status             string    `gorm:"comment:记录状态" json:"status"`
	StartTime          time.Time `gorm:"comment:开始时间" json:"startTime"`
	EndTime            time.Time `gorm:"comment:结束时间" json:"endTime"`
}

const ScheduleUniqueId = powermodel.UniqueId

const ScheduleStatusType = "_schedule_status" // 行程表状态

const ScheduleStatusIdle = "_idle"       // 空闲
const ScheduleStatusNormal = "_normal"   // 正常
const ScheduleStatusWarning = "_warning" // 警告
const ScheduleStatusFull = "_full"       // 已满

const StartWorkHour = 10
const BucketHours = 2
const BucketCount = 6

func (mdl *Schedule) LoadReservations(db *gorm.DB, conditions *map[string]interface{}, withClauseAssociations bool) {

	mdl.Reservations = []*Reservation{}
	err := powermodel.AssociationRelationship(db, conditions, mdl, "Reservations", false).Find(&mdl.Reservations)
	if err != nil {
		panic(errors.Wrap(err, "加载约单失败"))
	}

}

// 缓存数据字典
func (mdl *Schedule) GetCachedDDId(db *gorm.DB, itemType string, itemKey string) int {
	var item model.DataDictionaryItem
	if err := db.Model(item).
		//Debug().
		Where("type = ? AND key = ?", itemType, itemKey).First(&item).Error; err != nil {
		panic(err)
	}
	return int(item.Id)
}
