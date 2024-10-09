package wechat

import (
	"PowerX/internal/model"
	customerdomain2 "PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/model/powermodel"
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v3/security"
)

// 小程序客户信息
type WechatMPCustomer struct {
	powermodel.PowerModel

	Customer *customerdomain2.Customer `gorm:"foreignKey:OpenId;references:OpenIdInMiniProgram" json:"customer"`

	UniqueID   string `gorm:"unique" json:"uniqueId"`
	SessionKey string `json:"-"`
	OpenId     string `json:"openId"`
	UnionId    string `json:"unionId"`
	MPPhoneInfo
	MPUserInfo
	//PhoneNumber        string `json:"phoneNumber"`
	//PurePhoneNumber    string `json:"purePhoneNumber"`
	//CountryCode        string `json:"countryCode"`
	//WatermarkTimestamp int    `json:"watermarkTimestamp"`
	//WatermarkAppId     string `json:"watermarkAppId"`
	//NickName           string `json:"nickName"`
	//AvatarURL          string `json:"avatarUrl"`
	//Gender             string `json:"gender"`
	//Country            string `json:"country"`
	//Province           string `json:"province"`
	//City               string `json:"city"`
	//Language           string `json:"language"`
}

const WechatMpCustomerUniqueId = "unique_id"

func (mdl *WechatMPCustomer) TableName() string {
	return model.PowerXSchema + "." + model.TableNameWechatMPCustomer
}

func (mdl *WechatMPCustomer) GetTableName(needFull bool) string {
	tableName := model.TableNameWechatMPCustomer
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

// 小程序获取手机号
// https://developers.weixin.qq.com/miniprogram/dev/OpenApiDoc/user-info/phone-number/getPhoneNumber.html
type MPPhoneInfo struct {
	PhoneNumber     string `json:"phoneNumber"`
	PurePhoneNumber string `json:"purePhoneNumber"`
	CountryCode     string `json:"countryCode"`
	MPWater
}

type MPWater struct {
	Timestamp int    `gorm:"column:watermark_timestamp" json:"watermarkTimestamp"`
	Appid     string `gorm:"column:watermark_appid" json:"watermarkAppId"`
}

// 小程序客户信息
// https://developers.weixin.qq.com/miniprogram/dev/api/open-api/user-info/UserInfo.html
type MPUserInfo struct {
	NickName  string `json:"nickName,omitempty"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	Gender    string `json:"gender,omitempty"`
	Country   string `json:"country,omitempty"`
	Province  string `json:"province,omitempty"`
	City      string `json:"city,omitempty"`
	Language  string `json:"language,omitempty"`
}

func (mdl *WechatMPCustomer) GetComposedUniqueID() string {
	strKey := fmt.Sprintf("%s-%s", mdl.UniqueID, mdl.OpenId)
	hashKey := security.HashStringData(strKey)

	return hashKey
}
