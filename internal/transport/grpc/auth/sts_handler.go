package authgrpc

import (
	"context"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

type STSServiceServer struct {
	stsv1.UnimplementedSTSServiceServer
	issuer string
	ring   *KeyRing
}

func NewSTSServiceServer(deps *shared.Deps) *STSServiceServer {
	return NewSTSServiceServerWithRing(deps, NewDefaultKeyRing(deps))
}
func NewSTSServiceServerWithRing(_ *shared.Deps, ring *KeyRing) *STSServiceServer {
	return &STSServiceServer{
		issuer: "powerx.sts",
		ring:   ring,
	}
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
	// 1) 鉴别调用方（示例略过真实校验）
	var subject string
	if req.GetSubjectToken() != "" {
		subject = "subject_token"
	} else if req.GetClientId() != "" && req.GetClientSecret() != "" {
		subject = "client:" + req.GetClientId()
	} else {
		return &stsv1.ExchangeResponse{Meta: badMeta(ctx, 400, "missing subject_token or client credentials", req.GetCtx().GetRequestId())}, nil
	}

	// 2) audience/scope/ttl
	aud := req.GetAudience()
	if aud == "" {
		aud = "powerx:api"
	}
	scope := req.GetScope()
	ttl := time.Duration(req.GetTtlSeconds())
	if ttl <= 0 {
		ttl = 300
	}
	ttlDur := ttl * time.Second

	// 3) 代办主体 → CoreXClaims
	var tenantID, userID, memberID uint64
	if a := req.GetActor(); a != nil {
		tenantID = a.GetTenantId()
		switch a.GetKind() {
		case stsv1.ActorKind_ACTOR_KIND_USER:
			userID = a.GetUserId()
		case stsv1.ActorKind_ACTOR_KIND_CUSTOMER:
			memberID = a.GetMemberId()
		}
	}

	// 4) 选择 kid & key
	kid := s.ring.ResolveKIDForActor(req.GetActor())
	key := s.ring.GetHSByKID(kid)
	if len(key) == 0 {
		key = s.ring.GetHSByKID("default")
	}
	if len(key) == 0 {
		return &stsv1.ExchangeResponse{Meta: badMeta(ctx, 500, "no signing key", req.GetCtx().GetRequestId())}, nil
	}

	// 5) 构造并签发 JWT（HS256），写入 kid
	now := time.Now()
	exp := now.Add(ttlDur)

	claims := &reqctx.CoreXClaims{
		TenantID: tenantID,
		UserID:   userID,
		MemberID: memberID,
		Scope:    scope,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   subject,
			Audience:  []string{aud},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
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
