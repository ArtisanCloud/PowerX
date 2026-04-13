package backup_ops

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	intervalUnitMinute = "minute"
	intervalUnitHour   = "hour"
	intervalUnitDay    = "day"
)

var schedulePattern = regexp.MustCompile(`^(\d+)\s*([a-z]+)$`)

func parseScheduleDurationStrict(schedule string) (time.Duration, string, error) {
	raw := strings.TrimSpace(strings.ToLower(schedule))
	if raw == "" {
		return 0, "", ErrInvalidBackupPolicy
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d, raw, nil
	}
	match := schedulePattern.FindStringSubmatch(raw)
	if len(match) != 3 {
		return 0, "", ErrInvalidBackupPolicy
	}
	value, err := strconv.Atoi(match[1])
	if err != nil || value <= 0 {
		return 0, "", ErrInvalidBackupPolicy
	}
	unit, ok := normalizeIntervalUnit(match[2])
	if !ok {
		return 0, "", ErrInvalidBackupPolicy
	}
	return scheduleFromValueUnit(value, unit)
}

func scheduleFromValueUnit(value int, unit string) (time.Duration, string, error) {
	if value <= 0 {
		return 0, "", ErrInvalidBackupPolicy
	}
	normalizedUnit, ok := normalizeIntervalUnit(unit)
	if !ok {
		return 0, "", ErrInvalidBackupPolicy
	}
	switch normalizedUnit {
	case intervalUnitMinute:
		return time.Duration(value) * time.Minute, fmt.Sprintf("%dm", value), nil
	case intervalUnitHour:
		return time.Duration(value) * time.Hour, fmt.Sprintf("%dh", value), nil
	case intervalUnitDay:
		return time.Duration(value) * 24 * time.Hour, fmt.Sprintf("%dd", value), nil
	default:
		return 0, "", ErrInvalidBackupPolicy
	}
}

func normalizeIntervalUnit(unit string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(unit)) {
	case "m", "min", "mins", "minute", "minutes":
		return intervalUnitMinute, true
	case "h", "hr", "hrs", "hour", "hours":
		return intervalUnitHour, true
	case "d", "day", "days":
		return intervalUnitDay, true
	default:
		return "", false
	}
}

func durationToLegacyIntervalHours(d time.Duration) int {
	if d <= 0 {
		return defaultIntervalHours
	}
	hours := int(math.Ceil(d.Hours()))
	if hours < 1 {
		return 1
	}
	return hours
}
