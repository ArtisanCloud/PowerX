package system

import (
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// STSHandler 提供 Root 手工签发插件短期 Token 的能力（开发/调试场景使用）。
type STSHandler struct {
	cfg *config.Config
}

func NewSTSHandler(cfg *config.Config) *STSHandler {
	return &STSHandler{cfg: cfg}
}

// Mint 仅限 Root 调用：根据当前登录 Root 身份签发插件短期访问 Token。
func (h *STSHandler) Mint(c *gin.Context) {
	if h == nil || h.cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sts not initialized"})
		return
	}

	// ✅ 关键：按中间件规范，从 request.Context 读取 claims
	claims := reqctx.GetClaims(c.Request.Context())
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no auth"})
		return
	}
	if !claims.IsRoot { // 仅允许 Root 手工签发
		c.JSON(http.StatusForbidden, gin.H{"error": "root required"})
		return
	}

	pluginID := strings.TrimSpace(c.Param("pluginId"))
	if pluginID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing pluginId"})
		return
	}

	ttl := 10 * time.Minute
	if qs := strings.TrimSpace(c.Query("ttl")); qs != "" {
		if d, err := time.ParseDuration(qs); err == nil && d > 0 {
			if d > 24*time.Hour {
				d = 24 * time.Hour
			}
			ttl = d
		}
	}

	// 组装并签发（与网关 mint 逻辑保持一致：HS256 + issuer + audience + TTL + 宿主 secret）
	now := time.Now()
	subj := claims.MemberUUID
	if subj == "" {
		subj = claims.UserUUID
		if subj == "" {
			subj = "root"
		}
	}
	claimsCopy := *claims
	claimsCopy.Scope = "access"
	claimsCopy.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    h.cfg.Auth.Issuer,
		Subject:   subj,
		Audience:  []string{"plugin:" + pluginID},
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, &claimsCopy)
	signed, err := tok.SignedString([]byte(h.cfg.Auth.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "mint failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"plugin_id":    pluginID,
		"token_type":   "Bearer",
		"access_token": signed,
		"issuer":       h.cfg.Auth.Issuer,
		"audience":     "plugin:" + pluginID,
		"expires_in":   int(ttl.Seconds()),
		"expires_at":   now.Add(ttl).UTC().Format(time.RFC3339),
	})
}
