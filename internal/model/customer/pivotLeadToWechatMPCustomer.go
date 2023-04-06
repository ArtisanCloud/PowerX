package customer

import (
	"PowerX/internal/model/powermodel"
)

// Table Name
func (mdl *PivotCustomerToWechatMPCustomer) TableName() string {
	return TABLE_NAME_PIVOT_CUSTOMER_TO_WECHAT_MP_CUSTOMER
}

// 数据表结构
type PivotCustomerToWechatMPCustomer struct {
	*powermodel.PowerPivot

	CustomerID         int64 `gorm:"column:customer_id; not null;index:index_customer_id" json:"customerID"`
	WechatMPCustomerID int64 `gorm:"column:wechat_mp_customer_id; not null;index:index_wechat_mp_customer_id" json:"wechatMPCustomerID"`
}

const TABLE_NAME_PIVOT_CUSTOMER_TO_WECHAT_MP_CUSTOMER = "pivot_customer_to_wechat_mp_customer"
