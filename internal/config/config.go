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

type Config struct {
	Server         rest.RestConf
	JWTSecret      string
	PowerXDatabase Database

	WeWork WeWork
}
