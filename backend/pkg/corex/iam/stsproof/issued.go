package stsproof

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
)

var ErrUnavailable = errors.New("sts issuance store unavailable")
var ErrNotIssued = errors.New("sts token not issued by Core")

func key(id string) string {
	sum := sha256.Sum256([]byte(id))
	return "auth:sts:issued:v1:" + hex.EncodeToString(sum[:])
}
func fingerprint(c *reqctx.CoreXClaims) []byte {
	// 只绑定鉴权字段，不依赖 HTTP 中间件补全的数字 ID。
	raw, _ := json.Marshal([]any{c.ID, c.Issuer, c.Subject, c.PluginID, c.TenantUUID, c.Audience, c.Scope, c.IssuedAt, c.ExpiresAt, c.UserUUID, c.MemberUUID, c.IsRoot, c.Platforms})
	sum := sha256.Sum256(raw)
	return []byte(hex.EncodeToString(sum[:]))
}
func valid(c *reqctx.CoreXClaims) bool {
	if c == nil || c.Issuer != "powerx-sts" || c.PluginID == "" || c.ExpiresAt == nil || !time.Now().Before(c.ExpiresAt.Time) {
		return false
	}
	id, e := uuid.Parse(c.ID)
	return e == nil && id != uuid.Nil
}

// Record 仅由 Core 的 STS Exchange 在认证 client credentials 后调用。
func Record(ctx context.Context, store cache.ICache, c *reqctx.CoreXClaims) error {
	if store == nil {
		return ErrUnavailable
	}
	if !valid(c) {
		return ErrNotIssued
	}
	if store.Set(ctx, key(c.ID), fingerprint(c), time.Until(c.ExpiresAt.Time)) != nil {
		return ErrUnavailable
	}
	return nil
}
func Verify(ctx context.Context, store cache.ICache, c *reqctx.CoreXClaims) error {
	if store == nil {
		return ErrUnavailable
	}
	if !valid(c) {
		return ErrNotIssued
	}
	raw, e := store.Get(ctx, key(c.ID))
	if e != nil {
		return ErrUnavailable
	}
	if subtle.ConstantTimeCompare(raw, fingerprint(c)) != 1 {
		return ErrNotIssued
	}
	return nil
}
