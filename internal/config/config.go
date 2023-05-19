package config

import (
	"github.com/zeromicro/go-zero/rest"
)

type Database struct {
	DSN string
}

type WeWork struct {
	CropId         string
	AgentId        int
	Secret         string
	Token          string
	EncodingAESKey string
	HttpDebug      bool
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
	AESKey    string
	HttpDebug bool
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
	JWT    struct {
		JWTSecret    string
		MPJWTSecret  string
		WebJWTSecret string
	}

	PowerXDatabase Database
	Root           Root

	WechatOA      WechatOA
	WechatMP      WechatMP
	WechatPay     WechatPay
	WeWork        WeWork
	MediaResource MediaResource
}
