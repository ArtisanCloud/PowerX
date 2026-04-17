package plugin

// internal/transport/http/admin/plugin/i18n_loader.go

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	admdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gopkg.in/yaml.v3"
)

// 调试开关：需要详细日志就设为 true；上线建议 false
const i18nDebug = true

func dbgI18n(format string, args ...any) {
	if i18nDebug {
		logger.DebugF(context.Background(), "[i18n] "+format, args...)
	}
}

// ---- 环境路径 ----
const (
	envPluginInstalledRoot = "POWERX_PLUGIN_INSTALLED_ROOT"
	defaultInstalledRoot   = "plugins/installed"
)

// 入口：为“单个插件”加载菜单 i18n 包
// ☆ 关键增强：即使插件清单没声明 Frontend.Admin.I18n，也会按约定路径自动发现。
func loadPluginMenuI18n(ctx context.Context, p plugin_mgr.Plugin, requestedLocales []string) *admdto.MenuI18nPackage {
	// 1) 先拿清单上的目录
	var baseDir string
	if p.Frontend.Admin.I18n != nil && strings.TrimSpace(p.Paths.FrontendAdminI18nDir) != "" {
		baseDir = strings.TrimSpace(p.Paths.FrontendAdminI18nDir)
	}

	// 2) 兜底：<FrontendAdminDir>/i18n
	if baseDir == "" && strings.TrimSpace(p.Paths.FrontendAdminDir) != "" {
		try := filepath.Join(strings.TrimSpace(p.Paths.FrontendAdminDir), "i18n")
		if isDir(try) {
			baseDir = try
		}
	}

	// 3) 兜底：plugins/installed/<plugin-id>/**/web-admin/i18n
	if baseDir == "" {
		if guess := findInstalledI18nDir(p.ID); guess != "" {
			baseDir = guess
		}
	}

	if baseDir == "" || !isDir(baseDir) {
		dbgI18n("plugin=%s i18n baseDir not found", p.ID)
		return nil
	}
	dbgI18n("plugin=%s i18n baseDir=%s", p.ID, baseDir)

	// 4) 发现 locale 列表（清单优先，空则扫描目录）
	locales := make([]string, 0, 4)
	if p.Frontend.Admin.I18n != nil && len(p.Frontend.Admin.I18n.Locales) > 0 {
		locales = append(locales, p.Frontend.Admin.I18n.Locales...)
	} else {
		discovered, err := discoverLocales(baseDir)
		if err != nil {
			dbgI18n("list locales failed plugin=%s dir=%s err=%v", p.ID, baseDir, err)
			return nil
		}
		locales = append(locales, discovered...)
	}
	if len(locales) == 0 {
		return nil
	}

	// 5) locale 过滤：改为“宽松基码匹配”（zh 能命中 zh-CN）
	if len(requestedLocales) > 0 {
		locales = filterLocalesLoosely(locales, requestedLocales)
	}
	if len(locales) == 0 {
		return nil
	}
	sort.Strings(locales)
	dbgI18n("plugin=%s locales after filter=%v", p.ID, locales)

	// 6) 命名空间（清单优先，空则按文件名发现）
	var declaredNS []string
	if p.Frontend.Admin.I18n != nil && len(p.Frontend.Admin.I18n.Namespaces) > 0 {
		declaredNS = append(declaredNS, p.Frontend.Admin.I18n.Namespaces...)
	}

	resources := make(admdto.MenuI18nLocales)
	namespaceSet := map[string]struct{}{}

	for _, loc := range locales {
		loc = strings.TrimSpace(loc)
		if loc == "" {
			continue
		}
		localeDir := filepath.Join(baseDir, loc)
		if !isDir(localeDir) {
			continue
		}

		namespaces := append([]string(nil), declaredNS...)
		if len(namespaces) == 0 {
			discovered, err := discoverNamespaces(localeDir)
			if err != nil {
				dbgI18n("list namespaces failed plugin=%s locale=%s err=%v", p.ID, loc, err)
				continue
			}
			namespaces = discovered
		}
		if len(namespaces) == 0 {
			continue
		}

		sort.Strings(namespaces)
		nsData := make(admdto.MenuI18nNamespaces)
		for _, ns := range namespaces {
			ns = strings.TrimSpace(ns)
			if ns == "" {
				continue
			}
			filePath := resolveI18nFile(localeDir, ns, getFormat(p))
			if filePath == "" {
				dbgI18n("no i18n file plugin=%s locale=%s ns=%s", p.ID, loc, ns)
				continue
			}
			data, err := parseI18nFile(filePath)
			if err != nil {
				dbgI18n("parse failed plugin=%s locale=%s ns=%s err=%v", p.ID, loc, ns, err)
				continue
			}
			nsData[ns] = data
			namespaceSet[ns] = struct{}{}
			dbgI18n("loaded plugin=%s %s -> locale=%s ns=%s", p.ID, filePath, loc, ns)
		}
		if len(nsData) > 0 {
			resources[loc] = nsData
		}
	}

	if len(resources) == 0 {
		return nil
	}

	// 7) 汇总包
	namespaces := make([]string, 0, len(namespaceSet))
	for ns := range namespaceSet {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)

	format := getFormat(p)
	defNS := strings.TrimSpace(getDefaultNamespace(p))
	if defNS == "" {
		defNS = "menus" // 默认命名空间（你的文件就是 menus.json）
	}

	return &admdto.MenuI18nPackage{
		PluginID:         p.ID,
		Format:           format,
		DefaultNamespace: defNS,
		Namespaces:       namespaces,
		Locales:          resources,
	}
}

// --------- 辅助 ---------

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// 宽松 locale 过滤：en-US 接受 en；zh 接受 zh-CN/zh_TW
func filterLocalesLoosely(all, requested []string) []string {
	reqNorm := make([]string, 0, len(requested)*2)
	seen := map[string]struct{}{}
	add := func(s string) {
		s = normalizeLocale(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		reqNorm = append(reqNorm, s)
	}
	for _, r := range requested {
		add(r)
		if b := baseLang(r); b != "" {
			add(b)
		}
	}

	out := make([]string, 0, len(all))
	for _, loc := range all {
		n := normalizeLocale(loc)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			out = append(out, loc)
			continue
		}
		if b := baseLang(n); b != "" {
			if _, ok := seen[b]; ok {
				out = append(out, loc)
				continue
			}
		}
	}
	return out
}

func normalizeLocale(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "_", "-")
	return strings.ToLower(s)
}

func baseLang(s string) string {
	s = normalizeLocale(s)
	if i := strings.IndexByte(s, '-'); i > 0 {
		return s[:i]
	}
	return s
}

func getFormat(p plugin_mgr.Plugin) string {
	if p.Frontend.Admin.I18n != nil && strings.TrimSpace(p.Frontend.Admin.I18n.Format) != "" {
		return strings.TrimSpace(p.Frontend.Admin.I18n.Format)
	}
	return "i18next"
}

func getDefaultNamespace(p plugin_mgr.Plugin) string {
	if p.Frontend.Admin.I18n != nil && strings.TrimSpace(p.Frontend.Admin.I18n.DefaultNamespace) != "" {
		return strings.TrimSpace(p.Frontend.Admin.I18n.DefaultNamespace)
	}
	return ""
}

// 扫描：plugins/installed/<plugin-id>/**/web-admin/i18n
func findInstalledI18nDir(pluginID string) string {
	root := strings.TrimSpace(os.Getenv(envPluginInstalledRoot))
	if root == "" {
		root = defaultInstalledRoot
	}
	root = filepath.Join(root, pluginID)
	var hit string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() && isWebAdminI18nDir(p) {
			hit = p
			return filepath.SkipDir
		}
		return nil
	})
	return hit
}

func isWebAdminI18nDir(p string) bool {
	// 末尾是 /web-admin/i18n
	p = filepath.ToSlash(p)
	return strings.HasSuffix(p, "/web-admin/i18n")
}

// ---------- 目录/文件发现与解析（保持你的原实现风格） ----------

func discoverLocales(baseDir string) ([]string, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}
	locales := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		locales = append(locales, name)
	}
	return locales, nil
}

func discoverNamespaces(localeDir string) ([]string, error) {
	entries, err := os.ReadDir(localeDir)
	if err != nil {
		return nil, err
	}
	namespaces := make([]string, 0, len(entries))
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".json", ".yaml", ".yml":
		default:
			continue
		}
		base := strings.TrimSuffix(name, ext)
		if _, ok := seen[base]; ok {
			continue
		}
		seen[base] = struct{}{}
		namespaces = append(namespaces, base)
	}
	return namespaces, nil
}

func resolveI18nFile(localeDir, namespace, format string) string {
	for _, ext := range []string{".json", ".yaml", ".yml"} {
		path := filepath.Join(localeDir, namespace+ext)
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			return path
		}
	}
	return ""
}

func parseI18nFile(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	var out map[string]any
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
	default:
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
	}
	return normalizeI18nMap(out), nil
}

func normalizeI18nMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = normalizeI18nValue(v)
	}
	return out
}

func normalizeI18nValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return normalizeI18nMap(val)
	case map[interface{}]interface{}:
		converted := make(map[string]any, len(val))
		for k, vv := range val {
			converted[fmt.Sprint(k)] = normalizeI18nValue(vv)
		}
		return converted
	case []any:
		for i := range val {
			val[i] = normalizeI18nValue(val[i])
		}
		return val
	default:
		return val
	}
}
