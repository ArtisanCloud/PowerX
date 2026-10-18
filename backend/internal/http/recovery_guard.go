package http

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"io"
	"net/http"
	"strings"
)

// RecoveryMutationGuard 在验证或切换维护阶段禁止业务写入，登录只创建候选会话。
func RecoveryMutationGuard(enabled bool, prefix string) gin.HandlerFunc {
	prefix = "/" + strings.Trim(prefix, "/")
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}
		// WebSocket 以 GET 握手后仍可双向发送业务命令，不能沿用只读 GET 放行。
		if c.GetHeader("Upgrade") == "" && (c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions || c.Request.Method == http.MethodPost && recoveryReadOnlyPost(c.Request, prefix)) {
			c.Next()
			return
		}
		c.Header("Retry-After", "5")
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": "RECOVERY_READ_ONLY", "message": "系统正在恢复验证或维护，暂不接受业务写入"})
	}
}

// Only fixed, typed identity reads may use POST during verification. This is
// a maintenance gate, not authorization: the normal published capability,
// tenant registration, STS and live grant checks still run downstream.
func recoveryIdentityRead(r *http.Request) bool {
	if r.Body == nil {
		return false
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil || len(raw) > 64<<10 {
		return false
	}
	var input struct {
		CapabilityID      string `json:"capability_id"`
		PreferredProtocol string `json:"preferred_protocol"`
		Payload           struct {
			Body struct {
				Operation string          `json:"operation"`
				Request   json.RawMessage `json:"request"`
			} `json:"body"`
		} `json:"payload"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.PreferredProtocol != "core_internal" {
		return false
	}
	switch input.CapabilityID {
	case "com.corex.iam.identity_context.service_read":
		return input.Payload.Body.Operation == "resolve_request_identity"
	case "com.corex.iam.authorization_baseline.service_read":
		return input.Payload.Body.Operation == "read_member_baselines"
	}
	return false
}

func recoveryReadOnlyPost(r *http.Request, prefix string) bool {
	switch r.URL.Path {
	case prefix + "/admin/user/auth/login", prefix + "/admin/user/auth/refresh", prefix + "/admin/user/auth/logout", prefix + "/tenant/capabilities:grant-status":
		return true
	case prefix + "/tenant/invocations":
		return recoveryIdentityRead(r)
	case prefix + "/tenant/iam/members:batch", prefix + "/tenant/iam/members:batch-resolve":
		// Existing formal member-directory reads use POST only for a bounded UUID
		// set. The original STS, tenant and live permission checks remain in place.
		if r.Body == nil {
			return false
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
		r.Body = io.NopCloser(bytes.NewReader(raw))
		if err != nil || len(raw) > 64<<10 {
			return false
		}
		var input struct {
			MemberUUIDs []string `json:"member_uuids"`
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || len(input.MemberUUIDs) == 0 || len(input.MemberUUIDs) > 200 {
			return false
		}
		seen := map[uuid.UUID]bool{}
		for _, value := range input.MemberUUIDs {
			id, err := uuid.Parse(value)
			if err != nil || id == uuid.Nil || seen[id] {
				return false
			}
			seen[id] = true
		}
		return true
	}
	return false
}
