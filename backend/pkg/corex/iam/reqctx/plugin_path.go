package reqctx

import (
	"strings"
)

const defaultPluginIDPrefix = "com.powerx.plugins."

// ResolvePluginIDFromPath resolves plugin_id from known host routes.
// Supported:
// - /_p/<plugin_id>/...
// - /api/<version>/integration/<slug>/...
// - <apiPrefix>/integration/<slug>/...
func ResolvePluginIDFromPath(path string) string {
	return ResolvePluginIDFromPathWithAPIPrefix(path, "/api")
}

// ResolvePluginIDFromPathWithAPIPrefix resolves plugin_id with a configurable API prefix.
func ResolvePluginIDFromPathWithAPIPrefix(path, apiPrefix string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return ""
	}

	if strings.HasPrefix(p, "/_p/") {
		rest := strings.TrimPrefix(p, "/_p/")
		if rest == "" {
			return ""
		}
		if idx := strings.IndexByte(rest, '/'); idx > 0 {
			return strings.TrimSpace(rest[:idx])
		}
		return strings.TrimSpace(rest)
	}

	if rest, ok := integrationSlugPathRest(p, apiPrefix); ok {
		return normalizePluginIDFromSlug(rest)
	}

	return ""
}

func integrationSlugPathRest(path, apiPrefix string) (string, bool) {
	prefix := strings.TrimSpace(apiPrefix)
	if prefix != "" {
		if !strings.HasPrefix(prefix, "/") {
			prefix = "/" + prefix
		}
		prefix = strings.TrimRight(prefix, "/")
		integrationPrefix := prefix + "/integration/"
		if strings.HasPrefix(path, integrationPrefix) {
			return strings.TrimPrefix(path, integrationPrefix), true
		}
	}

	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 3 {
		return "", false
	}

	// /api/<version>/integration/<slug>/...
	if len(parts) >= 4 && parts[0] == "api" && strings.HasPrefix(strings.ToLower(parts[1]), "v") && parts[2] == "integration" {
		return strings.Join(parts[3:], "/"), true
	}
	// /<any-prefix>/integration/<slug>/...
	if parts[0] != "_p" && parts[1] == "integration" {
		return strings.Join(parts[2:], "/"), true
	}
	return "", false
}

func normalizePluginIDFromSlug(rest string) string {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return ""
	}
	slug := rest
	if idx := strings.IndexByte(rest, '/'); idx >= 0 {
		slug = rest[:idx]
	}
	slug = strings.TrimSpace(strings.ReplaceAll(slug, "_", "-"))
	if slug == "" {
		return ""
	}
	if strings.HasPrefix(slug, "com.") {
		return slug
	}
	return defaultPluginIDPrefix + slug
}
