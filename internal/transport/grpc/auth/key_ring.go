package authgrpc

import (
	"os"
	"strings"

	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
)

type KeyRing struct {
	hs map[string][]byte // kid -> HS256 secret
}

func NewKeyRing() *KeyRing { return &KeyRing{hs: make(map[string][]byte)} }

func (r *KeyRing) AddHS(kid string, key []byte) {
	kid = strings.TrimSpace(kid)
	if kid == "" || len(key) == 0 {
		return
	}
	r.hs[kid] = key
}
func (r *KeyRing) GetHSByKID(kid string) []byte { return r.hs[strings.TrimSpace(kid)] }
func (r *KeyRing) AllHS() [][]byte {
	out := make([][]byte, 0, len(r.hs))
	for _, k := range r.hs {
		out = append(out, k)
	}
	return out
}
func (r *KeyRing) FirstHS() []byte {
	for _, k := range r.hs {
		return k
	}
	return nil
}

// 按 Actor 决定 kid（你也可以换成按 audience 映射）
func (r *KeyRing) ResolveKIDForActor(a *stsv1.Actor) string {
	if a == nil {
		return "default"
	}
	switch a.GetKind() {
	case stsv1.ActorKind_ACTOR_KIND_USER:
		return "user"
	case stsv1.ActorKind_ACTOR_KIND_CUSTOMER:
		return "customer"
	case stsv1.ActorKind_ACTOR_KIND_SUPPLIER:
		return "supplier"
	default:
		return "default"
	}
}

// 从 deps 聚合密钥；支持环境变量补充 channel/supplier
// - user     -> deps.AuthUser.JWTSecret
// - customer -> deps.AuthCustomer.JWTSecret
// - channel  -> env POWERX_JWT_SECRET_CHANNEL
// - supplier -> env POWERX_JWT_SECRET_SUPPLIER
// - default  -> user（若存在）否则 customer，否则 POWERX_JWT_SECRET
func NewDefaultKeyRing(deps *shared.Deps) *KeyRing {
	r := NewKeyRing()
	if deps != nil && deps.AuthUser != nil && len(deps.AuthUser.JWTSecret) > 0 {
		r.AddHS("user", deps.AuthUser.JWTSecret)
		r.AddHS("default", deps.AuthUser.JWTSecret)
	}
	if deps != nil && deps.AuthCustomer != nil && len(deps.AuthCustomer.JWTSecret) > 0 {
		r.AddHS("customer", deps.AuthCustomer.JWTSecret)
		if r.GetHSByKID("default") == nil {
			r.AddHS("default", deps.AuthCustomer.JWTSecret)
		}
	}
	if v := strings.TrimSpace(os.Getenv("POWERX_JWT_SECRET_CHANNEL")); v != "" {
		r.AddHS("channel", []byte(v))
	}
	if v := strings.TrimSpace(os.Getenv("POWERX_JWT_SECRET_SUPPLIER")); v != "" {
		r.AddHS("supplier", []byte(v))
	}
	if r.GetHSByKID("default") == nil {
		if v := strings.TrimSpace(os.Getenv("POWERX_JWT_SECRET")); v != "" {
			r.AddHS("default", []byte(v))
		}
	}
	return r
}
