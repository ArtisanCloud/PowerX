package manager

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

// TryInternalToken 尝试获取宿主为该插件注入的内部通信令牌（仅内存）
func TryInternalToken(mgr plugin_mgr.Manager, pluginID string) (string, bool) {
	if impl, ok := mgr.(*managerImpl); ok {
		impl.mu.RLock()
		defer impl.mu.RUnlock()
		t, ok2 := impl.tokens[pluginID]
		return t, ok2
	}
	return "", false
}

func MintPluginAccessToken(mgr plugin_mgr.Manager, pluginID string) (string, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return "", errors.New("plugin_id is required")
	}
	impl, ok := mgr.(*managerImpl)
	if !ok || impl == nil || impl.opts.CoreConfig == nil {
		return "", errors.New("plugin manager core config is required")
	}
	cfg := impl.opts.CoreConfig.Auth
	secret := strings.TrimSpace(cfg.JWTSecret)
	issuer := strings.TrimSpace(cfg.Issuer)
	if secret == "" {
		return "", errors.New("auth.jwt_secret is required for plugin access token")
	}
	if issuer == "" {
		return "", errors.New("auth.issuer is required for plugin access token")
	}
	ttl := 15 * time.Minute
	if raw := strings.TrimSpace(cfg.AccessTTLStr); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return "", fmt.Errorf("invalid auth.access_ttl %q", raw)
		}
		ttl = parsed
	}
	claims := reqctx.CoreXClaims{
		MemberUUID: "plugin-enable",
		UserUUID:   "plugin-enable",
		IsRoot:     true,
		Roles:      []string{string(iam.CodeSystemAdmin)},
		Platforms:  []string{"plugin-enable"},
	}
	return auth.GenerateAccessJWT(claims, issuer, []string{"plugin:" + pluginID}, ttl, []byte(secret))
}

func TryRuntimeProcesses(mgr plugin_mgr.Manager, pluginID string) ([]RuntimeProcessView, bool) {
	if impl, ok := mgr.(*managerImpl); ok {
		return impl.RuntimeProcesses(pluginID), true
	}
	return nil, false
}
