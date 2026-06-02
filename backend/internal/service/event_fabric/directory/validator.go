package directory

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	namespacePattern = regexp.MustCompile(`^(_topic|[a-z][a-z0-9_-]*)(\.[a-z][a-z0-9_-]*)*$`)
	namePattern      = regexp.MustCompile(`^[a-z][a-z0-9-_]*$`)
	templateToken    = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)
)

func validateCreateInput(input CreateTopicInput) error {
	if strings.TrimSpace(input.TenantUUID) == "" {
		return fmt.Errorf("tenant_uuid is required")
	}
	if strings.TrimSpace(input.Namespace) == "" {
		return fmt.Errorf("namespace is required")
	}
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("name is required")
	}
	namespace, err := normalizeTemplateForValidation(strings.ToLower(input.Namespace))
	if err != nil {
		return fmt.Errorf("namespace: %w", err)
	}
	name, err := normalizeTemplateForValidation(strings.ToLower(input.Name))
	if err != nil {
		return fmt.Errorf("name: %w", err)
	}
	if !namespacePattern.MatchString(namespace) {
		return fmt.Errorf("namespace must match %s", namespacePattern.String())
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("name must match %s", namePattern.String())
	}
	return nil
}

func normalizeTemplateForValidation(value string) (string, error) {
	var tokenErr error
	normalized := templateToken.ReplaceAllStringFunc(value, func(match string) string {
		parts := templateToken.FindStringSubmatch(match)
		if len(parts) != 2 || !isAllowedTemplateToken(parts[1]) {
			tokenErr = fmt.Errorf("unsupported template token %s", strings.Trim(match, "{} "))
			return match
		}
		return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(parts[1])), ".", "_")
	})
	if tokenErr != nil {
		return "", tokenErr
	}
	if strings.Contains(normalized, "{{") || strings.Contains(normalized, "}}") {
		return "", fmt.Errorf("invalid template token")
	}
	return normalized, nil
}

func isAllowedTemplateToken(token string) bool {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "member_uuid", "member.uuid", "thread_id", "thread.id":
		return true
	default:
		return false
	}
}
