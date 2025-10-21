package manager

import (
	"errors"
	"regexp"
	"strings"
)

var (
	slugPattern      = regexp.MustCompile(`^[a-z0-9-]{3,63}$`)
	channelAllowlist = map[string]struct{}{
		"http": {},
		"mcp":  {},
	}
	scopeAllowlist = map[string]struct{}{
		"per_route":            {},
		"per_tenant":           {},
		"per_route_per_tenant": {},
	}
)

func validateSlug(slug string) error {
	if slug == "" {
		return errors.New("route_slug is required")
	}
	if !slugPattern.MatchString(slug) {
		return errors.New("route_slug must match ^[a-z0-9-]{3,63}$")
	}
	return nil
}

func normalizeSlug(slug string) string {
	return strings.ToLower(strings.TrimSpace(slug))
}

func normalizeChannels(channels []string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, c := range channels {
		clean := strings.ToLower(strings.TrimSpace(c))
		if clean == "" {
			continue
		}
		if _, ok := channelAllowlist[clean]; !ok {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	if len(result) == 0 {
		return []string{"http"}
	}
	return result
}

func normalizeToolGrants(toolGrants []string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, item := range toolGrants {
		clean := strings.TrimSpace(item)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	return result
}

func normalizeScope(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return "per_route_per_tenant"
	}
	if _, ok := scopeAllowlist[scope]; !ok {
		return "per_route_per_tenant"
	}
	return scope
}

func validateRateLimit(policy RateLimitPolicy) error {
	if policy.Limit == 0 {
		return errors.New("rate_limit.limit must be > 0")
	}
	if policy.WindowSeconds <= 0 {
		return errors.New("rate_limit.window_seconds must be > 0")
	}
	return nil
}
