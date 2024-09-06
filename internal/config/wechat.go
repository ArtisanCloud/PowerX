package config

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
