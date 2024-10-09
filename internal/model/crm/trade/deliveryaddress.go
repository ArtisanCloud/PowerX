package trade

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
)

// 订单发货地址
type DeliveryAddress struct {
	*powermodel.PowerModel

	OrderId      int64  `gorm:"comment:订单Id; index" json:"orderId"`
	CustomerId   int64  `gorm:"comment:客户Id; index" json:"customerId"`
	Recipient    string `gorm:"comment:收件人姓名" json:"recipient"`
	Name         string `gorm:"comment:地址名称" json:"name"`
	AddressLine  string `gorm:"comment:地址第一行" json:"addressLine"`
	AddressLine2 string `gorm:"comment:地址第二行" json:"addressLine2"`
	Street       string `gorm:"comment:街道地址" json:"street"`
	City         string `gorm:"comment:城市" json:"city"`
	Province     string `gorm:"comment:省份" json:"province"`
	PostalCode   string `gorm:"comment:邮政编码" json:"postalCode"`
	Country      string `gorm:"comment:国家" json:"country"`
	PhoneNumber  string `gorm:"comment:联系电话" json:"phoneNumber"`
}

func (mdl *DeliveryAddress) TableName() string {
	return model.PowerXSchema + "." + model.TableNameDeliveryAddress
}

func (mdl *DeliveryAddress) GetTableName(needFull bool) string {
	tableName := model.TableNameDeliveryAddress
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}
