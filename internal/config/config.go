package config

import (
	"PowerX/pkg/zerox"
	"github.com/zeromicro/go-zero/rest"
)

const DriverPostgres = "postgres"
const DriverMysql = "mysql"

type Database struct {
	Driver           string
	DSN              string
	SeedCommerceData bool
	SeedDepartment   bool
}

type RedisBase struct {
	Host     string
	Password string
}

type WeWork struct {
	CropId         string
	AgentId        int
	Secret         string
	Token          string
	EncodingAESKey string
	OAuth          struct {
		Callback string
		Scopes   []string
	}
	HttpDebug bool
	Debug     bool
}

type WechatOA struct {
	AppId  string
	Secret string
	AESKey string
	OAuth  struct {
		Callback string
		Scopes   []string
	}
	HttpDebug bool
	Debug     bool
}

type WechatPay struct {
	AppId            string
	MchId            string
	MchApiV3Key      string
	Key              string
	CertPath         string
	KeyPath          string
	RSAPublicKeyPath string
	SerialNo         string
	WechatPaySerial  string
	NotifyUrl        string
	HttpDebug        bool
	Debug            bool
}

type WechatMP struct {
	AppId  string
	Secret string
	AESKey string
	OAuth  struct {
		Callback string
		Scopes   []string
	}
	HttpDebug bool
	Debug     bool
}

type MediaResource struct {
	LocalStorage struct {
		StoragePath string
	}
	OSS struct {
		Enable bool
		Minio  struct {
			Endpoint    string
			Credentials struct {
				AccessKey string
				SecretKey string
			}
			UseSSL bool
		}
	}
}

type Root struct {
	Account  string
	Password string
	Name     string
}

type Config struct {
	Server rest.RestConf
	EtcDir string `json:",optional"`
	Log    zerox.LogConf
	Cors   Cors
	JWT    struct {
		JWTSecret    string
		MPJWTSecret  string
		WebJWTSecret string
	}

	PowerXDatabase Database
	RedisBase      RedisBase
	Root           Root

	WechatOA      WechatOA
	WechatMP      WechatMP
	WechatPay     WechatPay
	WeWork        WeWork
	MediaResource MediaResource
}
