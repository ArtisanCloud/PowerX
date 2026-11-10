package manager

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/redisnamespace"
)

// applyHostRedisNamespace owns installed-plugin Redis bindings. Package defaults,
// previous-version seeds and runtime.event_bridge cannot override the host scope.
func (m *managerImpl) applyHostRedisNamespace(env map[string]string, spec map[string]any, pluginID string) error {
	if m == nil || m.opts.CoreConfig == nil {
		return nil
	}
	cfg := m.opts.CoreConfig
	namespace, err := cfg.RedisNamespace()
	if err != nil {
		return fmt.Errorf("plugin Redis namespace: %w", err)
	}
	prefix := redisnamespace.Key(namespace, "plugin:"+pluginID+":")
	if env != nil {
		env["POWERX_ENVIRONMENT_ID"] = namespace
		env["POWERX_REDIS_KEY_PREFIX"] = prefix
	}
	if spec == nil {
		return nil
	}
	setNestedValue(spec, []string{"host", "environment_id"}, namespace)
	setNestedValue(spec, []string{"host", "redis", "key_prefix"}, prefix)
	// Preserve taskbus_provider=host. These fields scope the plugin's explicit
	// internal Redis streams, including older installed AI Craft versions.
	paths := [][]string{{"event_bridge"}}
	if runtime, ok := spec["runtime"].(map[string]any); ok {
		if _, exists := runtime["event_bridge"]; exists {
			paths = append(paths, []string{"runtime", "event_bridge"})
		}
	}
	for _, path := range paths {
		setNestedValue(spec, append(append([]string{}, path...), "redis_stream"), prefix+"powerx.taskbus.events")
		setNestedValue(spec, append(append([]string{}, path...), "redis_group"), prefix+"powerx.plugin")
		setNestedValue(spec, append(append([]string{}, path...), "taskbus_provider"), "host")
		setNestedValue(spec, append(append([]string{}, path...), "enabled"), true)
		setNestedValue(spec, append(append([]string{}, path...), "mode"), "taskbus")
	}
	// License 等业务缓存由插件的 Framework Cache 合同管理；宿主只提供通用隔离信息。
	if cfg.Cache.Driver != "redis" {
		return nil
	}
	address := net.JoinHostPort(strings.TrimSpace(cfg.Cache.Host), strconv.Itoa(cfg.Cache.Port))
	connection := &url.URL{Scheme: "redis", Host: address, Path: "/" + strconv.Itoa(cfg.Cache.DB)}
	if cfg.Cache.Password != "" {
		connection.User = url.UserPassword("default", cfg.Cache.Password)
	}
	for _, path := range paths {
		setNestedValue(spec, append(append([]string{}, path...), "redis_url"), connection.String())
	}
	return nil
}
