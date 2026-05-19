package reqctx

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
// CopyCtxToGin: 把标准 context 的常用键“影射”一份到 gin.Context.Keys，方便有些地方只拿 gin 的场景
func CopyCtxToGin(c *gin.Context) {
	ctx := c.Request.Context()
	if ctx == nil {
		return
	}
	// 仅拷贝常用的几个（不做深复制）
	if v := ctx.Value(KeyTenantID); v != nil {
		c.Set(string(KeyTenantID), v)
	}
	if v := ctx.Value(KeyTenantUUID); v != nil {
		c.Set(string(KeyTenantUUID), v)
	}
	if v := ctx.Value(KeyUserID); v != nil {
		c.Set(string(KeyUserID), v)
	}
	if v := ctx.Value(KeyMemberID); v != nil {
		c.Set(string(KeyMemberID), v)
	}
	if v := ctx.Value(KeyIsRoot); v != nil {
		c.Set(string(KeyIsRoot), v)
	}
	if v := ctx.Value(KeyEnv); v != nil {
		c.Set(string(KeyEnv), v)
	}
	if v := ctx.Value(KeyTraceID); v != nil {
		c.Set(string(KeyTraceID), v)
	}
	if v := ctx.Value(KeyRequestPath); v != nil {
		c.Set(string(KeyRequestPath), v)
	}
	if v := ctx.Value(KeyClaims); v != nil {
		c.Set(string(KeyClaims), v)
	}
}

// RequireTenantIDFromGin: 直接从 gin.Context 的 request context 里取（不中则返回错误）
func RequireTenantIDFromGin(c *gin.Context) (uint64, error) {
	return RequireTenantID(c.Request.Context())
}

func TenantUUIDFromGin(c *gin.Context) string {
	rc := c.Request.Context()
	if v := GetTenantUUID(rc); v != "" {
		return v
	}
	if v, ok := c.Get(string(KeyTenantUUID)); ok {
		if s, ok2 := v.(string); ok2 && s != "" {
			return s
		}
	}
	return ""
}

func RequireTenantUUIDFromGin(c *gin.Context) (string, error) {
	return RequireTenantUUID(c.Request.Context())
}

func TenantUUIDValueFromGin(c *gin.Context) uuid.UUID {
	return TenantUUIDValue(c.Request.Context())
}

func RequireTenantUUIDValueFromGin(c *gin.Context) (uuid.UUID, error) {
	return RequireTenantUUIDValue(c.Request.Context())
}
