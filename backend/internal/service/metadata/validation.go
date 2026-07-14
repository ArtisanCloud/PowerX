package metadata

import (
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var machineIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

var (
	ErrInvalidMachineIdentifier = errors.New("metadata.invalid_machine_identifier")
	ErrMissingRequiredLocale    = errors.New("metadata.missing_required_locale")
	ErrInvalidStatus            = errors.New("metadata.invalid_status")
)

func ValidateMachineIdentifier(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || !machineIDPattern.MatchString(value) {
		return ErrInvalidMachineIdentifier
	}
	if _, err := uuid.Parse(value); err == nil {
		return ErrInvalidMachineIdentifier
	}
	return nil
}

func ValidateRequiredI18n(values map[string]string, requiredLocale string) error {
	if strings.TrimSpace(requiredLocale) == "" {
		requiredLocale = "zh-CN"
	}
	if strings.TrimSpace(values[requiredLocale]) == "" {
		return ErrMissingRequiredLocale
	}
	return nil
}

func ValidateStatus(status string) error {
	switch status {
	case "enabled", "disabled", "archived":
		return nil
	default:
		return ErrInvalidStatus
	}
}
