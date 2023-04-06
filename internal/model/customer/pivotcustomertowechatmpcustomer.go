package customer

import (
	"PowerX/internal/model/powermodel"
)

// Table Name
func (mdl *PivotLeadToWechatMPCustomer) TableName() string {
	return TABLE_NAME_PIVOT_LEAD_TO_WECHAT_MP_CUSTOMER
}

// 数据表结构
type PivotLeadToWechatMPCustomer struct {
	*powermodel.PowerPivot

	LeadID             int64 `gorm:"column:lead_id; not null;index:index_lead_id" json:"leadID"`
	WechatMPCustomerID int64 `gorm:"column:wechat_mp_lead_id; not null;index:index_wechat_mp_lead_id" json:"wechatMPCustomerID"`
}

const TABLE_NAME_PIVOT_LEAD_TO_WECHAT_MP_CUSTOMER = "pivot_lead_to_wechat_mp_customer"
