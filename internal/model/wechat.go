package model

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v3/security"
	"gorm.io/datatypes"
)

// model
// 公众号的客户信息
// https://developers.weixin.qq.com/doc/offiaccount/User_Management/Get_users_basic_information_UnionId.html#UinonId
type WechatOACustomer struct {
	Customer *customerdomain2.Customer `gorm:"foreignKey:OpenId;references:OpenIdInWeChatOfficialAccount" json:"customer"`

	Model
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

// 小程序客户信息
type WechatMPCustomer struct {
	Customer *customerdomain2.Customer `gorm:"foreignKey:OpenId;references:OpenIdInMiniProgram" json:"customer"`

	Model
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
