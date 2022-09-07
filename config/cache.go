package config

import (
	"errors"
)

type CacheConfig struct {
	Default          string           `yaml:"default" json:"default"`
	CacheConnections CacheConnections `yaml:"connections" json:"connections"`
}

type CacheConnections struct {
	RedisConfig  RedisConfig  `yaml:"redis" json:"redis"`
	MemoryConfig MemoryConfig `yaml:"memory" json:"memory"`
}

type CacheBaseConfig struct {
	MaxIdle        int `yaml:"max_idle" json:"max_idle"`
	MaxActive      int `yaml:"max_active" json:"max_active"`
	Expiration     int `yaml:"expiration" json:"expiration"`
	TimeoutConnect int `yaml:"timeout_connect" json:"timeout_connect"`
	TimeoutRead    int `yaml:"timeout_read" json:"timeout_read"`
	TimeoutWrite   int `yaml:"timeout_write" json:"timeout_write"`
	TimeoutIdle    int `yaml:"timeout_idle" json:"timeout_idle"`
}

func LoadCacheConfig() (err error) {

	G_RedisConfig = &G_AppConfigure.CacheConfig.CacheConnections.RedisConfig

	err = parseCacheConfig()

	return err
}

func parseCacheConfig() (err error) {

	if G_RedisConfig == nil {
		return errors.New("加载缓存配置对象失败")
	}

	return err
}
