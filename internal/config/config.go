package config

import (
	"PowerX/pkg/zerox"
	"github.com/zeromicro/go-zero/rest"
)

const DriverPostgres = "postgres"
const DriverMysql = "mysql"

type Root struct {
	Account  string
	Password string
	Name     string
}

type Config struct {
	Version string
	Server  rest.RestConf
	EtcDir  string `json:",optional"`
	Log     zerox.LogConf
	Cors    Cors
	JWT     struct {
		JWTSecret    string
		MPJWTSecret  string
		WebJWTSecret string
	}
	OpenAPI OpenAPI

	PowerXDatabase Database
	RedisBase      RedisBase
	Root           Root

	WechatOA      WechatOA
	WechatMP      WechatMP
	WechatPay     WechatPay
	WeWork        WeWork
	MediaResource MediaResource
}
