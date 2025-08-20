// http/admin/auth_handler.go
package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	authsvc "github.com/ArtisanCloud/PowerX/pkg/corex/iam/service"
)

// ---------- DTO ----------

type RegisterReq struct {
	TenantID uint64 `json:"tenant_id"     binding:"required"`
	Username string `json:"username"      binding:"required,min=3,max=64"` // 租户内唯一，后端统一转小写
	Password string `json:"password"      binding:"required,min=6,max=64"`

	// 登录标识（二选一或都填；若都为空则回退用 username 作为 identifier）
	Email string `json:"email"         binding:"omitempty,email"`
	Phone string `json:"phone"         binding:"omitempty"`

	// 可选资料（优先写到 Member 作为租户内覆盖；未填时可回退到 username）
	DisplayName string `json:"display_name"  binding:"omitempty,max=128"`
	AvatarURL   string `json:"avatar_url"    binding:"omitempty,url"`
}

type LoginReq struct {
	TenantID   uint64 `json:"tenant_id"  binding:"required"`
	Identifier string `json:"identifier" binding:"required"` // username/email/phone 都行（我们按 username 处理）
	Password   string `json:"password"   binding:"required"`
}

type RefreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type LoginResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"` // "Bearer"
	ExpiresIn    int64  `json:"expires_in"` // access 的过期秒数
	Scope        string `json:"scope"`      // "access"
}

// ---------- Handlers ----------

func RegisterHandler(s *authsvc.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RegisterReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// 归一化
		username := strings.ToLower(strings.TrimSpace(req.Username))
		email := strings.ToLower(strings.TrimSpace(req.Email))
		phone := strings.TrimSpace(req.Phone)

		// 选择 identifier：优先 email，其次 phone，否则回退 username
		identifier := username
		if email != "" {
			identifier = email
		} else if phone != "" {
			identifier = phone
		}

		opt := &authsvc.RegisterOptions{
			UserEmail:         email,           // 写到全局 User（可选）
			UserPhone:         phone,           // 写到全局 User（可选）
			MemberDisplayName: req.DisplayName, // 租户内覆盖昵称（可选）
			MemberAvatarURL:   req.AvatarURL,   // 租户内覆盖头像（可选）
		}

		m, err := s.Register(c.Request.Context(), req.TenantID, username, identifier, req.Password, opt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"tenant_id":    m.TenantID,
			"member_id":    m.ID,
			"user_id":      m.UserID,
			"username":     m.Username,
			"display_name": m.DisplayName,
			"avatar_url":   m.AvatarURL,
		})
	}
}

func LoginHandler(s *authsvc.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		access, refresh, err := s.Login(c.Request.Context(), req.TenantID, req.Identifier, req.Password)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, LoginResp{
			AccessToken:  access,
			RefreshToken: refresh,
			TokenType:    "Bearer",
			ExpiresIn:    int64(s.AccessTTL.Seconds()),
			Scope:        "access",
		})
	}
}

func RefreshHandler(s *authsvc.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RefreshReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		access, err := s.Refresh(c.Request.Context(), req.RefreshToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"access_token": access,
			"token_type":   "Bearer",
			"expires_in":   int64(s.AccessTTL.Seconds()),
			"scope":        "access",
		})
	}
}

func LogoutHandler(s *authsvc.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RefreshReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := s.Logout(c.Request.Context(), req.RefreshToken); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
