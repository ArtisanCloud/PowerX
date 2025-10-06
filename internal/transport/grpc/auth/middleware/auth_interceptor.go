package middleware

import (
	"context"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

type KeyResolver interface {
	GetHSByKID(kid string) []byte
	AllHS() [][]byte
	FirstHS() []byte
}

type JWTVerifyConfig struct {
	Ring KeyResolver
}

func UnaryAuth(c JWTVerifyConfig) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = attachClaimsToCtx(ctx, c.Ring)
		return handler(ctx, req)
	}
}
func StreamAuth(c JWTVerifyConfig) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := attachClaimsToCtx(ss.Context(), c.Ring)
		wrapped := &serverStreamWithCtx{ServerStream: ss, ctx: ctx}
		return handler(srv, wrapped)
	}
}

type serverStreamWithCtx struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *serverStreamWithCtx) Context() context.Context { return w.ctx }

func attachClaimsToCtx(ctx context.Context, ring KeyResolver) context.Context {
	md, _ := metadata.FromIncomingContext(ctx)
	var tok string
	if vals := md.Get("authorization"); len(vals) > 0 {
		tok = vals[0]
	}
	if strings.HasPrefix(strings.ToLower(tok), "bearer ") {
		tok = strings.TrimSpace(tok[7:])
	} else {
		tok = ""
	}
	if tok == "" {
		return ctx
	}

	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))
	// A) 优先 kid
	t, err := parser.ParseWithClaims(tok, &reqctx.CoreXClaims{}, func(token *jwt.Token) (interface{}, error) {
		if kidRaw, ok := token.Header["kid"]; ok {
			if kid, ok := kidRaw.(string); ok && kid != "" {
				if k := ring.GetHSByKID(kid); len(k) > 0 {
					return k, nil
				}
			}
		}
		if k := ring.FirstHS(); len(k) > 0 {
			return k, nil
		}
		return []byte(""), nil
	})
	var claims *reqctx.CoreXClaims
	if err == nil && t != nil && t.Valid {
		if c, ok := t.Claims.(*reqctx.CoreXClaims); ok {
			claims = c
		}
	}
	// B) 兜底多密钥尝试
	if claims == nil {
		for _, k := range ring.AllHS() {
			if tt, e := parser.ParseWithClaims(tok, &reqctx.CoreXClaims{}, func(token *jwt.Token) (interface{}, error) { return k, nil }); e == nil && tt != nil && tt.Valid {
				if c, ok := tt.Claims.(*reqctx.CoreXClaims); ok {
					claims = c
					break
				}
			}
		}
	}
	if claims != nil {
		ctx = reqctx.WithClaims(ctx, claims)
		if claims.TenantID > 0 {
			ctx = reqctx.WithTenantID(ctx, claims.TenantID)
		}
		if claims.UserID > 0 {
			ctx = reqctx.WithUserID(ctx, claims.UserID)
		}
		if claims.MemberID > 0 {
			ctx = reqctx.WithMemberID(ctx, claims.MemberID)
		}
		if claims.Env != "" {
			ctx = reqctx.WithEnv(ctx, claims.Env)
		}
	}
	return ctx
}
