package config

import (
	"errors"
)

type CacheConfig struct {
	Default          string           `yaml:"default"`
	CacheConnections CacheConnections `yaml:"connections"`
}

type CacheConnections struct {
	RedisConfig  RedisConfig  `yaml:"redis"`
	MemoryConfig MemoryConfig `yaml:"memory"`
}

type CacheBaseConfig struct {
	MaxIdle        int `yaml:"max_idle"`
	MaxActive      int `yaml:"max_active"`
	Expiration     int `yaml:"expiration"`
	TimeoutConnect int `yaml:"timeout_connect"`
	TimeoutRead    int `yaml:"timeout_read"`
	TimeoutWrite   int `yaml:"timeout_write"`
	TimeoutIdle    int `yaml:"timeout_idle"`
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
