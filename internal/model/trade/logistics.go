package trade

import (
	"PowerX/internal/model/powermodel"
	"time"
)

type Logistics struct {
	*powermodel.PowerModel

	OrderId               int64           `gorm:"comment:订单Id" json:"orderId"`
	Status                LogisticsStatus `gorm:"comment:物流状态" json:"status"`
	TrackingCode          string          `gorm:"comment:物流追踪号" json:"trackingCode"`
	Carrier               string          `gorm:"comment:物流承运商" json:"carrier"`
	EstimatedDeliveryDate time.Time       `gorm:"comment:预计送达时间" json:"estimatedDeliveryDate"`
	ActualDeliveryDate    time.Time       `gorm:"comment:实际送达时间" json:"actualDeliveryDate"`
}

type LogisticsStatus string

const (
	LogisticsStatusPending   LogisticsStatus = "pending"    // 待发货
	LogisticsStatusInTransit LogisticsStatus = "in_transit" // 运输中
	LogisticsStatusDelivered LogisticsStatus = "delivered"  // 已送达
	LogisticsStatusCancelled LogisticsStatus = "cancelled"  // 已取消
	LogisticsStatusFailed    LogisticsStatus = "failed"     // 运输失败
	LogisticsStatusReturned  LogisticsStatus = "returned"   // 已退回
)
