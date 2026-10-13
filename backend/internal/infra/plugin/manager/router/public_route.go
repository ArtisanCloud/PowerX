package router

import (
	"net/url"
	"strings"
)

// NormalizePublicRoutePath 将已展开的 exposure 或网关 client path 转为插件 HTTP base 相对路径。
// 空路径或含歧义路径段时返回空字符串，不通过 path.Clean 折叠层级扩大公开范围。
func NormalizePublicRoutePath(value, httpBase string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	base := strings.TrimRight(strings.TrimSpace(httpBase), "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	if !validPublicRoutePath(value) || (base != "" && !validPublicRoutePath(base)) {
		return ""
	}
	// 复用既有 base 及去掉 /api 后的挂载别名规则，不改受保护路径的处理方式。
	return normalizeGatePathForPolicy(value, base)
}

func validPublicRoutePath(value string) bool {
	if value == "/" {
		return true
	}
	for _, segment := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil || decoded == "" || decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/\\?#\x00\r\n") {
			return false
		}
	}
	return true
}

func matchPublicRoutePath(pattern, reqPath string) bool {
	pattern = NormalizePublicRoutePath(pattern, "")
	reqPath = NormalizePublicRoutePath(reqPath, "")
	if pattern == "" || reqPath == "" {
		return false
	}
	patternParts := strings.Split(pattern, "/")
	requestParts := strings.Split(reqPath, "/")
	if len(patternParts) != len(requestParts) {
		return false
	}
	for i, part := range patternParts {
		parameter := (strings.HasPrefix(part, ":") && len(part) > 1) ||
			(strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") && len(part) > 2 && !strings.ContainsAny(part[1:len(part)-1], "{}"))
		if parameter {
			if requestParts[i] == "" {
				return false
			}
		} else if part != requestParts[i] {
			return false
		}
	}
	return true
}
