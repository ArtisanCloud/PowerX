package http

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// GenerateTokenRequest JWT令牌生成请求结构
type GenerateTokenRequest struct {
	TenantID string `json:"tenant_id" binding:"required"`
	Subject  string `json:"subject" binding:"required"`
	Platform string `json:"platform"`
	Audience string `json:"audience"`
	Scope    string `json:"scope"`
	TTL      int    `json:"ttl_hours"` // 小时数
}

// GenerateTokenHandler JWT令牌生成处理器
func GenerateTokenHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req GenerateTokenRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "detail": err.Error()})
			return
		}

		// 设置默认值
		setDefaultValues(&req)

		// 生成JWT令牌
		token, err := auth.GenerateJWT(
			req.TenantID,
			req.Subject,
			req.Platform,
			req.Audience,
			req.Scope,
			time.Duration(req.TTL)*time.Hour,
			[]byte(cfg.Auth.JWTSecret),
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "生成令牌失败", "detail": err.Error()})
			return
		}

		// 返回令牌信息
		c.JSON(http.StatusOK, gin.H{
			"token":      token,
			"tenant_id":  req.TenantID,
			"subject":    req.Subject,
			"platform":   req.Platform,
			"audience":   req.Audience,
			"scope":      req.Scope,
			"expires_in": req.TTL * 3600, // 秒数
			"usage":      fmt.Sprintf("curl -X POST http://localhost:%d/api/start_flow -H \"Authorization: Bearer %s\" -H \"Content-Type: application/json\"", cfg.Server.Port, token),
		})
	}
}

// setDefaultValues 设置请求的默认值
func setDefaultValues(req *GenerateTokenRequest) {
	if req.Platform == "" {
		req.Platform = "web"
	}
	if req.Audience == "" {
		req.Audience = "admin"
	}
	if req.Scope == "" {
		req.Scope = "flow:execute"
	}
	if req.TTL == 0 {
		req.TTL = 24 // 默认24小时
	}
}
