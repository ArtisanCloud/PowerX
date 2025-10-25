package cache

// pkg/cache/factory.go

import (
	"errors"
	"github.com/redis/go-redis/v9"
	"strconv"
)

func NewCache(cfg *CacheConfig) (ICache, error) {
	switch cfg.Driver {
	case "redis":
		c := redis.NewClient(&redis.Options{
			Addr:     cfg.Host + ":" + strconv.Itoa(cfg.Port),
			Password: cfg.Password,
			DB:       cfg.DB,
		})
		return NewRedisCache(c), nil

	default:
		return nil, errors.New("invalid cache driver")
	}
}
