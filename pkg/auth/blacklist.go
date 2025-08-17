package auth

import (
	"sync"
	"time"
)

// 简易jti黑名单（内存+TTL）
var bl = struct {
	sync.Mutex
	data map[string]time.Time
}{data: make(map[string]time.Time)}

// Revoke 撤销JWT令牌（将jti加入黑名单）
func Revoke(jti string, ttl time.Duration) {
	bl.Lock()
	defer bl.Unlock()
	bl.data[jti] = time.Now().Add(ttl)
}

// IsRevoked 检查JWT令牌是否已被撤销
func IsRevoked(jti string) bool {
	bl.Lock()
	defer bl.Unlock()
	exp, ok := bl.data[jti]
	if !ok {
		return false
	}
	// 如果已过期，从黑名单中删除
	if time.Now().After(exp) {
		delete(bl.data, jti)
		return false
	}
	return true
}

// CleanupExpired 清理过期的黑名单条目
func CleanupExpired() {
	bl.Lock()
	defer bl.Unlock()
	now := time.Now()
	for jti, exp := range bl.data {
		if now.After(exp) {
			delete(bl.data, jti)
		}
	}
}

// GetBlacklistSize 获取黑名单大小（用于监控）
func GetBlacklistSize() int {
	bl.Lock()
	defer bl.Unlock()
	return len(bl.data)
}
