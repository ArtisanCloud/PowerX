package workflow

import (
	"errors"
	"strings"
)

func normalizeTenantUUID(raw string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", errors.New("tenant uuid is required")
	}
	return trimmed, nil
}
