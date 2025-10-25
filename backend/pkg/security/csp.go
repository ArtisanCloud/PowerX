// pkg/security/csp.go （或你习惯的 util 位置）
package security

import (
	"net/url"
	"regexp"
	"slices"
	"strings"

	cfgpkg "github.com/ArtisanCloud/PowerX/config"
)

var hostSourceWildcard = regexp.MustCompile(`^\*?(\.[A-Za-z0-9.-]+)$`) // 匹配类似 *.powerx.io

func BuildFrameAncestorsCSP() string {
	cfg := cfgpkg.GetGlobalConfig()
	// 基础：永远包含 'self'
	lst := []string{"'self'"}
	seen := map[string]struct{}{"'self'": {}}

	for _, raw := range cfg.Security.FrameAncestors {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		// 允许：'self'、scheme-source（如 https:）、host-source（带或不带端口）、通配子域（如 https://*.powerx.io）
		// NOTE: frame-ancestors 不支持 '*' 任意源！
		if s == "'self'" {
			if _, ok := seen[s]; !ok {
				lst = append(lst, s)
				seen[s] = struct{}{}
			}
			continue
		}
		if strings.HasSuffix(s, ":") {
			// scheme-source（如 https:）——谨慎使用
			if _, ok := seen[s]; !ok {
				lst = append(lst, s)
				seen[s] = struct{}{}
			}
			continue
		}
		// host-source（可含通配子域）
		// 允许形如 https://host[:port] 或 http://*.domain
		if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
			// 粗校验
			if u, err := url.Parse(s); err == nil && u.Scheme != "" && u.Host != "" {
				if _, ok := seen[s]; !ok {
					lst = append(lst, s)
					seen[s] = struct{}{}
				}
				continue
			}
		}
		// 允许纯 host 通配（CSP 语法也支持不带 scheme 的 host-source，但推荐显式带上）
		// 比如：*.powerx.io（如果你要支持无 scheme，可在这里进一步放开）
		if hostSourceWildcard.MatchString(s) {
			if _, ok := seen[s]; !ok {
				lst = append(lst, s)
				seen[s] = struct{}{}
			}
			continue
		}
		// 其它无效项忽略（也可选择报错）
	}

	// 稳定排序（可选）
	slices.Sort(lst)
	return "frame-ancestors " + strings.Join(lst, " ")
}
