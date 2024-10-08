package wechat

import (
	"PowerX/internal/model"
	customerdomain2 "PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/model/powermodel"
	"gorm.io/datatypes"
)

// model
// 公众号的客户信息
// https://developers.weixin.qq.com/doc/offiaccount/User_Management/Get_users_basic_information_UnionId.html#UinonId
type WechatOACustomer struct {
	powermodel.PowerModel

	Customer *customerdomain2.Customer `gorm:"foreignKey:OpenId;references:OpenIdInWeChatOfficialAccount" json:"customer"`

	Subscribe      int            `json:"subscribe"`
	SessionKey     string         `json:"-"`
	OpenId         string         `json:"openId"`
	UnionId        string         `json:"unionId"`
	Language       string         `json:"language"`
	SubscribeTime  int            `json:"subscribeTime"`
	Remark         string         `json:"remark"`
	GroupId        int            `json:"groupId"`
	TagIdList      datatypes.JSON `json:"tagIdList"`
	SubscribeScene string         `json:"subscribeScene"`
	QrScene        int            `json:"qrScene"`
	QrSceneStr     string         `json:"qrSceneStr"`
}

func (mdl *WechatOACustomer) TableName() string {
	return model.PowerXSchema + "." + model.TableNameWechatOACustomer
}

func (mdl *WechatOACustomer) GetTableName(needFull bool) string {
	tableName := model.TableNameWechatOACustomer
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}
