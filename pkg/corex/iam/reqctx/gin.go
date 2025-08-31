package reqctx

import (
	"github.com/gin-gonic/gin"
)

// 从 gin.Context 读取 —— 统一先读 request.Context (JwtMW 已注入)，再兼容 gin.Context.Set 的副本

func TenantIDFromGin(c *gin.Context) *uint64 {
	rc := c.Request.Context()
	if id := GetTenantID(rc); id > 0 {
		return &id
	}
	if v, ok := c.Get(string(KeyTenantID)); ok {
		if id, ok2 := v.(uint64); ok2 && id > 0 {
			return &id
		}
	}
	return nil
}
func RequireTenantIDFromGin(c *gin.Context) (uint64, error) {
	if id := TenantIDFromGin(c); id != nil {
		return *id, nil
	}
	return 0, ErrTenantMissing
}

func EnvFromGin(c *gin.Context) string {
	rc := c.Request.Context()
	if e := GetEnv(rc); e != "" {
		return e
	}
	if v, ok := c.Get(string(KeyEnv)); ok {
		if s, ok2 := v.(string); ok2 && s != "" {
			return s
		}
	}
	return ""
}
func RequireEnvFromGin(c *gin.Context) (string, error) {
	if e := EnvFromGin(c); e != "" {
		return e, nil
	}
	return "", ErrEnvMissing
}

// 兼容：把 request.Context 的关键值复制到 gin.Context（如果你的老代码还在 c.Get(...)）
func CopyCtxToGin(c *gin.Context) {
	rc := c.Request.Context()
	if v := GetTenantID(rc); v > 0 {
		c.Set(string(KeyTenantID), v)
	}
	if v := GetTenantUUID(rc); v != "" {
		c.Set(string(KeyTenantUUID), v)
	}
	if v := GetEnv(rc); v != "" {
		c.Set(string(KeyEnv), v)
	}
	if v := GetEnvWhitelist(rc); len(v) > 0 {
		c.Set(string(KeyEnvs), v)
	}
	if v := GetUserID(rc); v > 0 {
		c.Set(string(KeyUserID), v)
	}
	if v := GetMemberID(rc); v > 0 {
		c.Set(string(KeyMemberID), v)
	}
	if v := IsRoot(rc); v {
		c.Set(string(KeyIsRoot), v)
	}
	if cl := GetClaims(rc); cl != nil {
		c.Set(string(KeyClaims), cl)
	}
}
