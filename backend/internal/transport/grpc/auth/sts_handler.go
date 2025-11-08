package authgrpc

import (
    "context"
    "errors"
    "strings"
    "time"

    "github.com/golang-jwt/jwt/v5"

    commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
    stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"

    "github.com/ArtisanCloud/PowerX/internal/app/shared"
    "github.com/ArtisanCloud/PowerX/internal/service/setting"
    "github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

type STSServiceServer struct {
    stsv1.UnimplementedSTSServiceServer
    issuer string
    ring   *KeyRing
    cred   *setting.PluginInstanceConfigService
}

func NewSTSServiceServer(deps *shared.Deps) *STSServiceServer {
    return NewSTSServiceServerWithRing(deps, NewDefaultKeyRing(deps))
}
func NewSTSServiceServerWithRing(deps *shared.Deps, ring *KeyRing) *STSServiceServer {
    // 默认 issuer（gRPC 验证不校验 issuer，仅用于回显）
    issuer := "powerx-sts"
    srv := &STSServiceServer{issuer: issuer, ring: ring}
    if deps != nil {
        srv.cred = setting.NewPluginInstanceConfigService(deps)
    }
    return srv
}

/******** meta helpers ********/
func okMeta(ctx context.Context, reqID string) *commonv1.ResponseMeta {
	if reqID == "" {
		reqID = reqctx.GetTraceID(ctx)
	}
	return &commonv1.ResponseMeta{Code: 200, Message: "success", Timestamp: time.Now().Unix(), RequestId: reqID}
}
func badMeta(ctx context.Context, code int32, msg, reqID string) *commonv1.ResponseMeta {
	return &commonv1.ResponseMeta{Code: code, Message: msg, Timestamp: time.Now().Unix(), RequestId: reqID}
}

/******** Exchange ********/
func (s *STSServiceServer) Exchange(ctx context.Context, req *stsv1.ExchangeRequest) (*stsv1.ExchangeResponse, error) {
    // 1) 鉴别调用方：优先 client_credentials（plugin_id + tenant_id）
    var subject string
    var tenantID uint64
    var pluginID string
    if cid, sec := req.GetClientId(), req.GetClientSecret(); cid != "" && sec != "" {
        // 解析 client_id（约定：<pluginID>.<tenantID>）
        pID, tID, perr := parseClientID(cid)
        if perr != nil {
            return &stsv1.ExchangeResponse{Meta: badMeta(ctx, 400, "invalid client_id format", req.GetCtx().GetRequestId())}, nil
        }
        pluginID, tenantID = pID, tID

        // 校验凭证与能力约束
        if s.cred == nil { return &stsv1.ExchangeResponse{Meta: badMeta(ctx, 500, "sts not initialized", req.GetCtx().GetRequestId())}, nil }
        wantAud := req.GetAudience()
        wantScope := req.GetScope()
        if err := s.cred.VerifyClient(ctx, tenantID, pluginID, cid, sec, wantAud, wantScope, ""); err != nil {
            return &stsv1.ExchangeResponse{Meta: badMeta(ctx, 401, "invalid client credentials", req.GetCtx().GetRequestId())}, nil
        }
        subject = "client:" + cid
    } else if req.GetSubjectToken() != "" {
        // 预留：基于 subject_token 的交换（未实现）
        subject = "subject_token"
    } else {
        return &stsv1.ExchangeResponse{Meta: badMeta(ctx, 400, "missing subject_token or client credentials", req.GetCtx().GetRequestId())}, nil
    }

	// 2) audience/scope/ttl
    aud := req.GetAudience()
    if aud == "" { aud = "powerx:api" }
    scope := req.GetScope()
    if scope == "" { scope = "access" }
    ttl := time.Duration(req.GetTtlSeconds())
    if ttl <= 0 {
        ttl = 300
    }
    ttlDur := ttl * time.Second

    // 3) 代办主体 → CoreXClaims
    if a := req.GetActor(); a != nil {
        // 允许覆盖来自 client_id 的 tenantID（如需）
        if a.GetTenantId() > 0 { tenantID = a.GetTenantId() }
        switch a.GetKind() {
        case stsv1.ActorKind_ACTOR_KIND_USER:
            userID := a.GetUserId()
            _ = userID
        case stsv1.ActorKind_ACTOR_KIND_CUSTOMER:
            memberID := a.GetMemberId()
            _ = memberID
        }
    }

    // 4) 构造并签发 JWT（使用 KeyRing 的 HS256 key，并附带 kid，便于 gRPC 拦截器验证）
    now := time.Now()
    claims := &reqctx.CoreXClaims{
        TenantID: tenantID,
        Scope:    scope,
        RegisteredClaims: jwt.RegisteredClaims{
            Issuer:   s.issuer,
            Subject:  subject,
            Audience: []string{aud},
            IssuedAt: jwt.NewNumericDate(now),
            ExpiresAt: jwt.NewNumericDate(now.Add(ttlDur)),
        },
    }
    // 选择 kid & key：按 Actor 决定 kid；若无则取 default
    kid := s.ring.ResolveKIDForActor(req.GetActor())
    key := s.ring.GetHSByKID(kid)
    if len(key) == 0 { kid = "default"; key = s.ring.GetHSByKID("default") }
    if len(key) == 0 { return &stsv1.ExchangeResponse{Meta: badMeta(ctx, 500, "no signing key", req.GetCtx().GetRequestId())}, nil }
    tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    tok.Header["kid"] = kid
    ss, err := tok.SignedString(key)
    if err != nil {
        return &stsv1.ExchangeResponse{Meta: badMeta(ctx, 500, "sign failed: "+err.Error(), req.GetCtx().GetRequestId())}, nil
    }

	return &stsv1.ExchangeResponse{
		Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
		Data: &stsv1.ExchangeData{
			AccessToken: ss,
			TokenType:   "Bearer",
			ExpiresIn:   int64(ttlDur.Seconds()),
			Audience:    aud,
			Scope:       scope,
			Issuer:      s.issuer,
			Subject:     subject,
			IssuedAt:    now.Unix(),
		},
	}, nil
}

// parseClientID: 约定 client_id = "<pluginID>.<tenantID>"
func parseClientID(cid string) (pluginID string, tenantID uint64, err error) {
    parts := strings.Split(cid, ".")
    if len(parts) < 2 { return "", 0, errors.New("invalid") }
    last := parts[len(parts)-1]
    var tid uint64
    for i := 0; i < len(last); i++ { if last[i] < '0' || last[i] > '9' { return "", 0, errors.New("invalid") } }
    // 简单无依赖转换
    for i := 0; i < len(last); i++ { tid = tid*10 + uint64(last[i]-'0') }
    if tid == 0 { return "", 0, errors.New("invalid") }
    return strings.Join(parts[:len(parts)-1], "."), tid, nil
}

/******** Introspect（可选） ********/
func (s *STSServiceServer) Introspect(ctx context.Context, req *stsv1.IntrospectRequest) (*stsv1.IntrospectResponse, error) {
	claims, _ := s.parseByKID(req.GetAccessToken())
	if claims == nil {
		return &stsv1.IntrospectResponse{Meta: okMeta(ctx, ""), Data: &stsv1.IntrospectData{Active: false}}, nil
	}
	aud := ""
	if len(claims.Audience) > 0 {
		aud = claims.Audience[0]
	}
	return &stsv1.IntrospectResponse{
		Meta: okMeta(ctx, ""),
		Data: &stsv1.IntrospectData{
			Active:   true,
			Audience: aud,
			Issuer:   claims.Issuer,
			Subject:  claims.Subject,
			Exp:      claims.ExpiresAt.Time.Unix(),
			Scope:    claims.Scope,
		},
	}, nil
}

// 先按 header.kid 取 key 解析；失败则兜底多密钥尝试
func (s *STSServiceServer) parseByKID(tokenStr string) (*reqctx.CoreXClaims, error) {
	if strings.TrimSpace(tokenStr) == "" {
		return nil, jwt.ErrTokenMalformed
	}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))

	// 方案 A：一次 Parse，Keyfunc 根据 kid 决定 key
	t, err := parser.ParseWithClaims(tokenStr, &reqctx.CoreXClaims{}, func(token *jwt.Token) (interface{}, error) {
		if kidRaw, ok := token.Header["kid"]; ok {
			if kid, ok := kidRaw.(string); ok && kid != "" {
				if k := s.ring.GetHSByKID(kid); len(k) > 0 {
					return k, nil
				}
			}
		}
		// 没有 kid 就给一个任意 key（会失败，然后再走 B 兜底）
		if k := s.ring.FirstHS(); len(k) > 0 {
			return k, nil
		}
		return []byte(""), nil
	})
	if err == nil && t != nil {
		if c, ok := t.Claims.(*reqctx.CoreXClaims); ok && t.Valid {
			return c, nil
		}
	}

	// 方案 B：兜底多密钥尝试
	for _, k := range s.ring.AllHS() {
		if tt, e := parser.ParseWithClaims(tokenStr, &reqctx.CoreXClaims{}, func(token *jwt.Token) (interface{}, error) {
			return k, nil
		}); e == nil && tt != nil && tt.Valid {
			if c, ok := tt.Claims.(*reqctx.CoreXClaims); ok {
				return c, nil
			}
		}
	}
	return nil, jwt.ErrTokenInvalidClaims
}
