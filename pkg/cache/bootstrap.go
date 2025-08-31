package cache

import (
	"sync"
)

var (
	once        sync.Once
	instance    ICache
	instanceErr error
)

// InitCache 仅首次生效；之后的调用直接返回同一个实例（和同一个错误）
// 若你需要拿到初始化时的错误，调用这个而不是 GetCache。
func InitCache(cfg *CacheConfig) (ICache, error) {
	once.Do(func() {
		instance, instanceErr = NewCache(cfg)
	})
	return instance, instanceErr
}

// GetCache 在已初始化后随处读取；若未初始化，将返回 nil。
func GetCache() ICache {
	return instance
}
