package plugin

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

func loadPluginMenuI18n(ctx context.Context, p plugin_mgr.Plugin, requestedLocales []string) *admdto.MenuI18nPackage {
	spec := p.Frontend.Admin.I18n
	if spec == nil {
		return nil
	}
	baseDir := strings.TrimSpace(p.Paths.FrontendAdminI18nDir)
	if baseDir == "" {
		return nil
	}

	locales := append([]string(nil), spec.Locales...)
	if len(locales) == 0 {
		discovered, err := discoverLocales(baseDir)
		if err != nil {
			logger.WarnF(ctx, "[plugin:i18n] 列出语言目录失败 plugin=%s dir=%s err=%v", p.ID, baseDir, err)
			return nil
		}
		locales = discovered
	}
	if len(requestedLocales) > 0 {
		reqSet := map[string]struct{}{}
		for _, loc := range requestedLocales {
			loc = strings.TrimSpace(loc)
			if loc == "" {
				continue
			}
			reqSet[loc] = struct{}{}
		}
		filtered := make([]string, 0, len(locales))
		for _, loc := range locales {
			if _, ok := reqSet[loc]; ok {
				filtered = append(filtered, loc)
			}
		}
		locales = filtered
	}
	if len(locales) == 0 {
		return nil
	}

	sort.Strings(locales)
	resources := make(admdto.MenuI18nLocales)
	namespaceSet := map[string]struct{}{}

	for _, locale := range locales {
		locale = strings.TrimSpace(locale)
		if locale == "" {
			continue
		}
		localeDir := filepath.Join(baseDir, locale)
		if fi, err := os.Stat(localeDir); err != nil || !fi.IsDir() {
			continue
		}

		namespaces := append([]string(nil), spec.Namespaces...)
		if len(namespaces) == 0 {
			discovered, err := discoverNamespaces(localeDir)
			if err != nil {
				logger.WarnF(ctx, "[plugin:i18n] 列出命名空间失败 plugin=%s locale=%s err=%v", p.ID, locale, err)
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
			filePath := resolveI18nFile(localeDir, ns, spec.Format)
			if filePath == "" {
				logger.WarnF(ctx, "[plugin:i18n] 未找到资源文件 plugin=%s locale=%s namespace=%s", p.ID, locale, ns)
				continue
			}
			data, err := parseI18nFile(filePath)
			if err != nil {
				logger.WarnF(ctx, "[plugin:i18n] 解析资源失败 plugin=%s locale=%s namespace=%s err=%v", p.ID, locale, ns, err)
				continue
			}
			nsData[ns] = data
			namespaceSet[ns] = struct{}{}
		}
		if len(nsData) > 0 {
			resources[locale] = nsData
		}
	}

	if len(resources) == 0 {
		return nil
	}

	namespaces := make([]string, 0, len(namespaceSet))
	for ns := range namespaceSet {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)

	format := strings.TrimSpace(spec.Format)
	if format == "" {
		format = "i18next"
	}

	return &admdto.MenuI18nPackage{
		PluginID:         p.ID,
		Format:           format,
		DefaultNamespace: spec.DefaultNamespace,
		Namespaces:       namespaces,
		Locales:          resources,
	}
}

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
	candidates := candidateExts(format)
	for _, ext := range candidates {
		path := filepath.Join(localeDir, namespace+ext)
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			return path
		}
	}
	return ""
}

func candidateExts(format string) []string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json", "nuxt":
		return []string{".json", ".yaml", ".yml"}
	case "i18next":
		return []string{".json", ".yaml", ".yml"}
	default:
		return []string{".json", ".yaml", ".yml"}
	}
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
