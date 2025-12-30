// api/http/admin/auth/auth_handler.go
package auth

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type AuthUserHandler struct {
	AuthService *authsvc.AuthService
}

func NewAuthUserHandler(deps *shared.Deps) *AuthUserHandler {
	return &AuthUserHandler{
		AuthService: deps.AuthUser,
	}
}

// ---------- DTO ----------

type RegisterReq struct {
	UserName string `json:"username"      binding:"required,min=3,max=64"` // 租户内唯一，后端统一转小写
	Password string `json:"password"      binding:"required,min=6,max=64"`

	// 登录标识（二选一或都填；若都为空则回退用 username 作为 identifier）
	Email string `json:"email"         binding:"omitempty,email"`
	Phone string `json:"phone"         binding:"omitempty"`

	// 可选资料（优先写到 Member 作为租户内覆盖；未填时可回退到 username）
	DisplayName string `json:"display_name"  binding:"omitempty,max=128"`
	AvatarURL   string `json:"avatar_url"    binding:"omitempty,url"`
}

type LoginReq struct {
	Tenant     string `json:"tenant,omitempty"`
	Identifier string `json:"identifier" binding:"required"` // 邮箱/手机号/用户名（三选一）
	Password   string `json:"password"   binding:"required"`
}

type RefreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type LoginResp struct {
	TokenType    string `json:"token_type"` // "Bearer"
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`    // access 的过期秒数
	RefreshToken string `json:"refresh_token"` // 首次登录返回；刷新时可不返
	Scope        string `json:"scope"`         // "access"
}

// ---------- Handlers ----------

func (h *AuthUserHandler) tenantUUIDFromGin(c *gin.Context) (string, bool) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return "", false
	}
	canonical, err := reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "tenant_uuid 无效", err)
		return "", false
	}
	return canonical, true
}

func (h *AuthUserHandler) RegisterHandler(s *authsvc.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RegisterReq
		if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
			dtoRequest.ResponseValidationError(c, err)
			return
		}
		tenantUUID, ok := h.tenantUUIDFromGin(c)
		if !ok {
			return
		}

		// 归一化
		username := strings.ToLower(strings.TrimSpace(req.UserName))
		email := strings.ToLower(strings.TrimSpace(req.Email))
		phone := strings.TrimSpace(req.Phone)

		// 选择 identifier：优先 email，其次 phone，否则回退 username
		identifier := chooseIdentifier(username, email, phone)

		opt := &authsvc.RegisterOptions{
			UserEmail:         email,           // 写到全局 User（可选）
			UserPhone:         phone,           // 写到全局 User（可选）
			MemberDisplayName: req.DisplayName, // 租户内覆盖昵称（可选）
			MemberAvatarURL:   req.AvatarURL,   // 租户内覆盖头像（可选）
		}

		m, err := s.Register(c.Request.Context(), tenantUUID, username, identifier, req.Password, opt)
		if err != nil {
			// 常见业务错误归为 400；如果你有更细的错误码可在这里分支
			dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
			return
		}

		dtoRequest.ResponseSuccess(c, gin.H{
			"tenant_uuid":  tenantUUID,
			"member_id":    m.ID,
			"user_id":      m.UserID,
			"username":     m.Username,
			"display_name": m.DisplayName,
			"avatar_url":   m.AvatarURL,
		})
	}
}

func (h *AuthUserHandler) LoginHandler(s *authsvc.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginReq
		if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
			dtoRequest.ResponseValidationError(c, err)
			return
		}

		identifier := strings.ToLower(strings.TrimSpace(req.Identifier))
		access, refresh, err := s.Login(c.Request.Context(), req.Tenant, identifier, req.Password)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusUnauthorized, err.Error(), nil)
			return
		}

		resp := &LoginResp{
			TokenType:    "Bearer",
			AccessToken:  access,
			ExpiresIn:    int(s.AccessTTL.Seconds()),
			RefreshToken: refresh,
			Scope:        "access",
		}
		dtoRequest.ResponseSuccess(c, resp)
	}
}

func (h *AuthUserHandler) RefreshHandler(s *authsvc.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RefreshReq
		if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
			dtoRequest.ResponseValidationError(c, err)
			return
		}
		access, err := s.Refresh(c.Request.Context(), req.RefreshToken)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusUnauthorized, err.Error(), nil)
			return
		}

		resp := &LoginResp{
			TokenType:   "Bearer",
			AccessToken: access,
			ExpiresIn:   int(s.AccessTTL.Seconds()),
			Scope:       "access",
		}
		dtoRequest.ResponseSuccess(c, resp)
	}
}

func (h *AuthUserHandler) LogoutHandler(s *authsvc.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RefreshReq // 用 refresh_token 注销
		if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
			dtoRequest.ResponseValidationError(c, err)
			return
		}
		if err := s.Logout(c.Request.Context(), req.RefreshToken); err != nil {
			dtoRequest.ResponseError(c, http.StatusUnauthorized, err.Error(), nil)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
	}
}

// ---------- helpers ----------

func chooseIdentifier(username, email, phone string) string {
	if email != "" {
		return email
	}
	if phone != "" {
		return phone
	}
	return username
}
