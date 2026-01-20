package websocket

import (
	"encoding/base64"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"strings"

	"github.com/gin-gonic/gin"
)

func b64urlDecode(s string) (string, error) {
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// BearerShim: 在鉴权前，把 ?authorization=Bearer xxx 或子协议 bearer.<b64url(jwt)> 提升成 Authorization
func BearerShim() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 已有 Authorization 则不动
		if c.GetHeader("Authorization") == "" {
			// 1) URL 参数兜底：?authorization=Bearer xxx
			if auth := c.Query("authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				c.Request.Header.Set("Authorization", auth)
			}

			// 2) 子协议：Sec-WebSocket-Protocol: bearer.<b64url(jwt)>
			if c.GetHeader("Authorization") == "" {
				// Values() 会自动按规范键名取值，兼容大小写/不同拼写
				vals := c.Request.Header.Values(contract.HeaderKeySecWebSocketProtocol)
				// 兼容只传单值的情况
				if len(vals) == 0 {
					if v := c.GetHeader("Sec-WebSocket-Protocol"); v != "" {
						vals = []string{v}
					}
				}
				for _, v := range vals {
					for _, p := range strings.Split(v, ",") { // 逗号分隔多个子协议
						pp := strings.TrimSpace(p)
						if strings.HasPrefix(strings.ToLower(pp), "bearer.") {
							raw := pp[strings.IndexByte(pp, '.')+1:]
							if tok, err := b64urlDecode(raw); err == nil && tok != "" {
								c.Request.Header.Set("Authorization", "Bearer "+tok)
								// 回显协议，握手时更标准
								c.Writer.Header().Set("Sec-WebSocket-Protocol", pp)
								goto NEXT
							}
						}
					}
				}
			}
		}
		if c.GetHeader("X-Tenant-UUID") == "" {
			if tenant := strings.TrimSpace(c.Query("tenant_uuid")); tenant != "" {
				c.Request.Header.Set("X-Tenant-UUID", tenant)
			}
		}
	NEXT:
		c.Next()
	}
}
