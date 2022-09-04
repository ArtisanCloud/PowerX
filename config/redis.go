package config

import (
	"github.com/ArtisanCloud/PowerLibs/v2/cache"
)

type RedisConfig struct {
	cache.CacheInterface
	CacheBaseConfig

	Protocol       string `yaml:"protocol"`
	Host           string `yaml:"host"`
	Password       string `yaml:"password"`
	DB             int    `yaml:"db"`
	SSLEnabled     bool   `yaml:"ssl_enabled"`
	TimeoutConnect int    `yaml:"timeout_connect"`
}
