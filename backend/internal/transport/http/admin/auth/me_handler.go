package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	dto "github.com/ArtisanCloud/PowerX/pkg/dto"

	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
)

type MeContextHandler struct {
	S *authsvc.MeService
}

func NewMeContextHandler(dep *shared.Deps) *MeContextHandler {
	return &MeContextHandler{
		S: dep.MeService,
	}
}

// GET /api/v1/auth/me/context
func (h *MeContextHandler) GetMeContext(c *gin.Context) {
	ctx := c.Request.Context()

	resp, err := h.S.GetMeContext(ctx)
	if err != nil {
		// 统一错误返回
		dto.ResponseError(c, http.StatusInternalServerError, "获取上下文失败", err)
		return
	}

	// 从请求头获取 PowerX 上下文并回传给前端（响应头 + cookie + JSON 字段）
	ctxVal := c.GetHeader("X-PowerX-CTX")
	ctxSig := c.GetHeader("X-PowerX-CTX-SIG")
	ctxJwt := c.GetHeader("X-PowerX-CTX-JWT")

	// 如果上游未注入且服务端有 JWTSecret，则基于 auth_claims 或 Bearer token 现算一份签名上下文
	if ctxVal == "" || ctxSig == "" {
		if secret := auth.GetJWTSecret(); len(secret) > 0 {
			var rawClaims []byte
			// 优先：auth_claims
			if claimsAny, ok := c.Get("auth_claims"); ok {
				rawClaims, _ = json.Marshal(claimsAny)
			}
			// 其次：Authorization Bearer
			if len(rawClaims) == 0 {
				if authz := c.GetHeader("Authorization"); strings.HasPrefix(strings.ToLower(authz), "bearer ") {
					parts := strings.Split(authz, ".")
					if len(parts) == 3 {
						if decoded, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
							rawClaims = decoded
						}
					}
				}
			}
			if len(rawClaims) > 0 {
				ctxVal = base64.StdEncoding.EncodeToString(rawClaims)
				mac := hmac.New(sha256.New, secret)
				mac.Write([]byte(ctxVal))
				ctxSig = base64.StdEncoding.EncodeToString(mac.Sum(nil))
			}
		}
	}

	if ctxJwt == "" {
		if authz := c.GetHeader("Authorization"); strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			ctxJwt = strings.TrimSpace(authz[len("Bearer "):])
		}
	}

	if ctxVal != "" {
		resp.Ctx = ctxVal
		c.Header("X-PowerX-CTX", ctxVal)
		// 设置可读 cookie，便于前端桥接读取
		c.SetCookie("px_ctx", ctxVal, 0, "/", "", false, false)
	}
	if ctxSig != "" {
		resp.CtxSig = ctxSig
		c.Header("X-PowerX-CTX-SIG", ctxSig)
		c.SetCookie("px_ctx_sig", ctxSig, 0, "/", "", false, false)
	}
	if ctxJwt != "" {
		resp.CtxJwt = ctxJwt
		c.Header("X-PowerX-CTX-JWT", ctxJwt)
		c.SetCookie("px_ctx_jwt", ctxJwt, 0, "/", "", false, false)
	}

	dto.ResponseSuccess(c, resp)
}
