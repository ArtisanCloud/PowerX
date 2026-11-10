package marketplace

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var asciiHost = regexp.MustCompile(`^[a-z0-9.-]+$`)

// NormalizePublicSite 只规范化挑战使用的 HTTPS 单入口；DNS/出站限制仍由抓取器实现。
func NormalizePublicSite(raw string) (string, error) {
	return normalizeWebsite(raw, true)
}

// NormalizeWebsite 用于普通网站识别；HTTP 不满足实例关联的 HTTPS 门槛。
func NormalizeWebsite(raw string) (string, error) { return normalizeWebsite(raw, false) }

func normalizeWebsite(raw string, requireHTTPS bool) (string, error) {
	if strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\\%\x00\r\n\t?#") {
		return "", fmt.Errorf("ambiguous site URL")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && (requireHTTPS || u.Scheme != "http")) || u.Host == "" || u.User != nil || u.Opaque != "" {
		return "", fmt.Errorf("invalid HTTPS site")
	}
	host := strings.ToLower(u.Hostname())
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	} else {
		if !asciiHost.MatchString(host) || !strings.Contains(host, ".") || strings.Contains(host, "..") || strings.Trim(host, "0123456789.") == "" {
			return "", fmt.Errorf("ambiguous hostname")
		}
		for _, label := range strings.Split(host, ".") {
			if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") || strings.HasPrefix(label, "0x") {
				return "", fmt.Errorf("invalid hostname")
			}
		}
	}
	port := u.Port()
	defaultPort := "443"
	if u.Scheme == "http" {
		defaultPort = "80"
	}
	if port != "" && port != defaultPort {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("invalid port")
		}
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	u.Host = host
	if u.Path == "" {
		u.Path = "/"
	}
	if u.EscapedPath() != u.Path || path.Clean(u.Path) != strings.TrimSuffix(u.Path, "/") && u.Path != "/" {
		return "", fmt.Errorf("ambiguous site path")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/"
	return u.String(), nil
}
