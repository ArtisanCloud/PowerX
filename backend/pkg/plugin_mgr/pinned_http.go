package plugin_mgr

import (
	"context"
	"fmt"
	"strings"
)

// PinnedHTTPIdentity 只由宿主进程管理器创建，不接受工具/provider 自报身份。
type PinnedHTTPIdentity struct {
	PluginID, Version, ImplementationSHA256, Generation string
}

// PinnedHTTPRequest 是受信任 adapter 的固定合同调用，不向模型开放 URL/headers。
type PinnedHTTPRequest struct {
	Method, Path string
	Headers      map[string]string
	Body         []byte
}

// PinnedHTTPRuntime 是 Manager 的可选端口；远程/开发挂载不能假装固定实现。
type PinnedHTTPRuntime interface {
	PinHTTP(context.Context, string, string, string) (PinnedHTTPIdentity, error)
	InvokePinnedHTTP(context.Context, PinnedHTTPIdentity, PinnedHTTPRequest) ([]byte, error)
}

// PluginHTTPRoute 将合同的插件相对路径映射到唯一的插件 HTTP base；不读取 Core API prefix。
func PluginHTTPRoute(base, resource string) (string, error) {
	base = strings.TrimSuffix(base, "/")
	for _, value := range []string{base, resource} {
		if value == "" {
			continue
		}
		if !strings.HasPrefix(value, "/") || strings.Contains(value, "//") || strings.ContainsAny(value, "?#%\\\r\n") {
			return "", fmt.Errorf("plugin.tool_path_invalid")
		}
		for _, segment := range strings.Split(value, "/") {
			if segment == "." || segment == ".." {
				return "", fmt.Errorf("plugin.tool_path_invalid")
			}
		}
	}
	if resource == "" || resource == "/" {
		return "", fmt.Errorf("plugin.tool_path_invalid")
	}
	return base + resource, nil
}

// CatalogPluginEndpoint 是既有目录坐标，不是实际网络地址。实际转发由 HTTP base 决定。
func CatalogPluginEndpoint(pluginID, channel, endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	lower := strings.ToLower(endpoint)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(endpoint, "/_p/") {
		return endpoint
	}
	if (channel != "http" && channel != "rest") || strings.TrimSpace(pluginID) == "" {
		return endpoint
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	return "/_p/" + strings.TrimSpace(pluginID) + endpoint
}
