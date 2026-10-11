package plugin_identity

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

var wsHandlePattern = regexp.MustCompile(`^pxwsi_v1_[A-Za-z0-9_-]{43}$`)

func (s *Service) wsKey(handle string) string {
	return "iam:ws-identity:v1:" + s.opts.Namespace + ":" + hash(handle)
}

func (s *Service) issueWSHandle(ctx context.Context, ticket string, p proof) (string, error) {
	// 原票据有 256 位随机熵；派生 handle 保持不透明，并让重复解析幂等。
	// TTL 总是原 access 的剩余寿命，绝不从本次复核时间重新延长。
	digest := sha256.Sum256([]byte("ws:" + s.opts.Namespace + ":" + ticket))
	handle := "pxwsi_v1_" + base64.RawURLEncoding.EncodeToString(digest[:])
	p.ExpiresAt = p.UserExpiresAt
	raw, err := json.Marshal(p)
	if err != nil || !s.clock().Before(p.ExpiresAt) {
		return "", failure(401, "IAM_CONTEXT_USER_SESSION_EXPIRED")
	}
	if err := s.store.Set(ctx, s.wsKey(handle), raw, p.ExpiresAt.Sub(s.clock())); err != nil {
		return "", failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	return handle, nil
}

// RevalidateWS 供 Framework 在授权业务消息前调用；不使用定时轮询。
func (s *Service) RevalidateWS(ctx context.Context, in WSQuery) (Result, error) {
	return s.revalidateWS(ctx, in, false)
}

// RevalidateWSAdmin 仅允许管理员用户检查自己同登录会话、同租户的 handle。
// 管理端检查不能签发 handle 或代替插件的 service grant。
func (s *Service) RevalidateWSAdmin(ctx context.Context, in WSQuery) (Result, error) {
	return s.revalidateWS(ctx, in, true)
}

func (s *Service) revalidateWS(ctx context.Context, in WSQuery, admin bool) (Result, error) {
	if !s.ready() {
		return Result{}, failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	tenant := canonical(reqctx.GetTenantUUID(ctx))
	if !admin {
		var err error
		tenant, err = s.serviceActor(ctx)
		if err != nil {
			return Result{}, err
		}
	} else {
		c := reqctx.GetClaims(ctx)
		if c == nil || cap.ServiceCredential(ctx) || c.Issuer != s.opts.UserIssuer || !has(c.Audience, s.opts.UserAudience) || tenant == "" {
			return Result{}, failure(401, "IAM_CONTEXT_USER_SESSION_INVALID")
		}
	}
	if in.Schema != WSQuerySchema {
		return Result{}, failure(400, "IAM_CONTEXT_INVALID_ARGUMENT")
	}
	if in.Handle == "" {
		return Result{}, failure(401, "IAM_CONTEXT_WS_HANDLE_REQUIRED")
	}
	if !wsHandlePattern.MatchString(in.Handle) {
		return Result{}, failure(401, "IAM_CONTEXT_WS_HANDLE_INVALID")
	}
	raw, err := s.store.Get(ctx, s.wsKey(in.Handle))
	if err != nil {
		return Result{}, failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	if len(raw) == 0 {
		return Result{}, failure(401, "IAM_CONTEXT_WS_HANDLE_EXPIRED")
	}
	var p proof
	if json.Unmarshal(raw, &p) != nil || p.Version != 1 || (p.Transport != "ws_grant" && p.Transport != "ws_upgrade") || !p.ExpiresAt.Equal(p.UserExpiresAt) {
		return Result{}, failure(503, "IAM_CONTEXT_UNAVAILABLE")
	}
	if !s.clock().Before(p.ExpiresAt) {
		return Result{}, failure(401, "IAM_CONTEXT_WS_HANDLE_EXPIRED")
	}
	if p.TenantUUID != tenant {
		return Result{}, failure(403, "IAM_CONTEXT_TENANT_FORBIDDEN")
	}
	if !admin {
		if err = s.owner(ctx, tenant, p.PluginID); err != nil {
			return Result{}, err
		}
	} else {
		c := reqctx.GetClaims(ctx)
		if canonical(c.UserUUID) != p.UserUUID || c.ID != p.SessionJTI {
			return Result{}, failure(403, "IAM_CONTEXT_USER_FORBIDDEN")
		}
	}
	out, err := s.resolveProof(ctx, p, admin)
	if err != nil {
		return Result{}, err
	}
	out.Schema = WSResultSchema
	out.WSRevalidationHandle = in.Handle
	out.WSRevalidationExpiresAt = p.ExpiresAt.UTC().Format(time.RFC3339Nano)
	return out, nil
}
