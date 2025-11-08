package delivery

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// CompatibilityMode 定义版本协商模式。
type CompatibilityMode string

const (
	CompatibilityModeAny      CompatibilityMode = "any"
	CompatibilityModeStrict   CompatibilityMode = "strict"
	CompatibilityModeBackward CompatibilityMode = "backward"
)

var versionPattern = regexp.MustCompile(`(?i)^v?(\d+)$`)

// NegotiationOutcome 描述版本协商结果。
type NegotiationOutcome struct {
	Compatible      bool
	Mode            CompatibilityMode
	SelectedVersion string
	Reason          string
}

// VersionNegotiator 抽象版本协商实现。
type VersionNegotiator interface {
	Negotiate(mode CompatibilityMode, eventVersion string, supported []string) NegotiationOutcome
}

// DefaultVersionNegotiator 默认实现。
type DefaultVersionNegotiator struct{}

// Negotiate 根据模式与订阅者支持的版本判断事件是否兼容。
func (n *DefaultVersionNegotiator) Negotiate(mode CompatibilityMode, eventVersion string, supported []string) NegotiationOutcome {
	normalizedMode := normalizeMode(mode)
	outcome := NegotiationOutcome{
		Compatible: false,
		Mode:       normalizedMode,
		Reason:     "",
	}

	switch normalizedMode {
	case CompatibilityModeAny:
		outcome.Compatible = true
		outcome.SelectedVersion = eventVersion
		return outcome
	case CompatibilityModeStrict:
		if len(supported) == 0 {
			outcome.Reason = "no supported versions declared"
			return outcome
		}
		for _, sv := range supported {
			if strings.EqualFold(strings.TrimSpace(sv), eventVersion) {
				outcome.Compatible = true
				outcome.SelectedVersion = eventVersion
				return outcome
			}
		}
		outcome.Reason = fmt.Sprintf("event version %s not in supported list", eventVersion)
		return outcome
	case CompatibilityModeBackward:
		if len(supported) == 0 {
			// 没有声明则视为无限制
			outcome.Compatible = true
			outcome.SelectedVersion = eventVersion
			return outcome
		}
		eventNumber, ok := normalizeVersionNumber(eventVersion)
		if !ok {
			// 未能解析版本号时回退到严格匹配
			for _, sv := range supported {
				if strings.EqualFold(strings.TrimSpace(sv), eventVersion) {
					outcome.Compatible = true
					outcome.SelectedVersion = eventVersion
					return outcome
				}
			}
			outcome.Reason = fmt.Sprintf("event version %s unsupported", eventVersion)
			return outcome
		}
		numbers := extractVersionNumbers(supported)
		if len(numbers) == 0 {
			// supported 中没有有效数字，回退严格模式
			for _, sv := range supported {
				if strings.EqualFold(strings.TrimSpace(sv), eventVersion) {
					outcome.Compatible = true
					outcome.SelectedVersion = eventVersion
					return outcome
				}
			}
			outcome.Reason = fmt.Sprintf("event version %s unsupported", eventVersion)
			return outcome
		}
		sort.Ints(numbers)
		maxSupported := numbers[len(numbers)-1]
		if eventNumber <= maxSupported {
			outcome.Compatible = true
			outcome.SelectedVersion = eventVersion
			return outcome
		}
		outcome.Reason = fmt.Sprintf("event version %s exceeds supported maximum v%d", eventVersion, maxSupported)
		return outcome
	default:
		// 未知模式按 any 处理
		outcome.Mode = CompatibilityModeAny
		outcome.Compatible = true
		outcome.SelectedVersion = eventVersion
		return outcome
	}
}

func normalizeMode(mode CompatibilityMode) CompatibilityMode {
	switch strings.ToLower(string(mode)) {
	case string(CompatibilityModeStrict):
		return CompatibilityModeStrict
	case string(CompatibilityModeBackward):
		return CompatibilityModeBackward
	case string(CompatibilityModeAny):
		return CompatibilityModeAny
	default:
		return CompatibilityModeAny
	}
}

func normalizeVersionNumber(version string) (int, bool) {
	matches := versionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if len(matches) != 2 {
		return 0, false
	}
	n, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

func extractVersionNumbers(versions []string) []int {
	numbers := make([]int, 0, len(versions))
	for _, v := range versions {
		if n, ok := normalizeVersionNumber(v); ok {
			numbers = append(numbers, n)
		}
	}
	return numbers
}
