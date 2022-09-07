package config

import (
	"github.com/ArtisanCloud/PowerLibs/v2/cache"
)

type RedisConfig struct {
	cache.CacheInterface
	CacheBaseConfig

	Protocol       string `yaml:"protocol" json:"protocol"`
	Host           string `yaml:"host" json:"host"`
	Password       string `yaml:"password" json:"password"`
	DB             int    `yaml:"db" json:"db"`
	SSLEnabled     bool   `yaml:"ssl_enabled" json:"ssl_enabled"`
	TimeoutConnect int    `yaml:"timeout_connect" json:"timeout_connect"`
}
