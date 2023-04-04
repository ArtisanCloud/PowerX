package config

import "github.com/zeromicro/go-zero/rest"

type Database struct {
	DSN string
}

type WeWork struct {
	CropId    string
	AgentId   int
	Secret    string
	HttpDebug bool
}

type WechatOA struct {
	AppId     string
	Secret    string
	HttpDebug bool
}

type WechatPay struct {
	AppId            string
	MchID            string
	MchApiV3Key      string
	Key              string
	CertPath         string
	KeyPath          string
	RSAPublicKeyPath string
	SerialNo         string
	HttpDebug        bool
}

type WechatMP struct {
	AppId     string
	Secret    string
	HttpDebug bool
}

type Config struct {
	Server         rest.RestConf
	JWTSecret      string
	PowerXDatabase Database

	WechatOA  WechatOA
	WechatMP  WechatMP
	WechatPay WechatPay
	WeWork    WeWork
}
