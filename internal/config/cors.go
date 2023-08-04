package config

import (
	fmt "PowerX/pkg/printx"
	"github.com/zeromicro/go-zero/rest"
)

type Cors struct {
	AllowAll        bool
	SupportedDomain []string
}

func SetupCors(conf *Config) rest.RunOption {

	// 判断跨域支持情况
	if conf.Cors.AllowAll {
		return rest.WithCors()
	} else if len(conf.Cors.SupportedDomain) > 0 {
		fmt.Dump(conf.Cors.SupportedDomain)
		return rest.WithCors(conf.Cors.SupportedDomain...)
	} else {
		fmt.Dump("not allow cors")
		return nil
	}
}
