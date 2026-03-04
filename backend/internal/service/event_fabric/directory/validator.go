package directory

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	namespacePattern = regexp.MustCompile(`^(_topic|[a-z][a-z0-9]*)(\.[a-z][a-z0-9]*)*$`)
	namePattern      = regexp.MustCompile(`^[a-z][a-z0-9-_]*$`)
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
	if !namespacePattern.MatchString(strings.ToLower(input.Namespace)) {
		return fmt.Errorf("namespace must match %s", namespacePattern.String())
	}
	if !namePattern.MatchString(strings.ToLower(input.Name)) {
		return fmt.Errorf("name must match %s", namePattern.String())
	}
	return nil
}
