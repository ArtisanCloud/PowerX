package model

import "gorm.io/datatypes"

// model
// 公众号的客户信息
// https://developers.weixin.qq.com/doc/offiaccount/User_Management/Get_users_basic_information_UnionID.html#UinonId
type WechatOACustomer struct {
	Model
	Subscribe      int            `json:"subscribe"`
	SessionKey     string         `json:"-"`
	OpenID         string         `json:"openid"`
	UnionID        string         `json:"unionid"`
	Language       string         `json:"language"`
	SubscribeTime  int            `json:"subscribe_time"`
	Remark         string         `json:"remark"`
	GroupID        int            `json:"groupid"`
	TagIDList      datatypes.JSON `json:"tagid_list"`
	SubscribeScene string         `json:"subscribe_scene"`
	QrScene        int            `json:"qr_scene"`
	QrSceneStr     string         `json:"qr_scene_str"`
}

// 小程序客户信息
type WechatMPCustomer struct {
	Model
	SessionKey string `json:"-"`
	OpenID     string `gorm:"unique" json:"openid"`
	UnionID    string `gorm:"unique" json:"unionid"`
	MPPhoneInfo
	MPUserInfo
	//PhoneNumber        string `json:"phoneNumber"`
	//PurePhoneNumber    string `json:"purePhoneNumber"`
	//CountryCode        string `json:"countryCode"`
	//WatermarkTimestamp int    `json:"watermarkTimestamp"`
	//WatermarkAppID     string `json:"watermarkAppID"`
	//NickName           string `json:"nickName"`
	//AvatarURL          string `json:"avatarUrl"`
	//Gender             string `json:"gender"`
	//Country            string `json:"country"`
	//Province           string `json:"province"`
	//City               string `json:"city"`
	//Language           string `json:"language"`
}

const WECHAT_MP_CUSTOMER_UNIQUE_ID = "open_id"

type FindMPCustomerOption struct {
	Ids             []int64
	SessionKey      string
	OpenIDs         []string
	UnionIDs        []string
	PhoneNumbers    []string
	PhoneNumberLike string
	NickNames       []string
	NickNameLike    string
	Gender          int64
	Country         string
	Province        string
	City            string
	//Statuses        []MPCustomerStatus
	PageIndex int
	PageSize  int
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
