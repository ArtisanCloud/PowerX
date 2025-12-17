package capability_registry

import "errors"

// ErrCapabilityNotFound 表示 Selector 无法解析目标能力。
var ErrCapabilityNotFound = ErrSelectorCapabilityRequired

// ErrCapabilityForbidden 表示能力不属于当前租户授权范围。
var ErrCapabilityForbidden = ErrSelectorCapabilityForbidden

// ErrSafeModeActive 表示租户处于 safe-mode，调用被阻断。
var ErrSafeModeActive = ErrSelectorSafeModeActive

// ErrToolGrantMissing 表示调用缺少必要的 Tool Grant。
var ErrToolGrantMissing = ErrSelectorToolGrantRequired

func isSelectorError(err error) bool {
	return errors.Is(err, ErrSelectorCapabilityRequired) ||
		errors.Is(err, ErrSelectorCapabilityForbidden) ||
		errors.Is(err, ErrSelectorTenantRequired) ||
		errors.Is(err, ErrSelectorSafeModeActive) ||
		errors.Is(err, ErrSelectorToolGrantRequired) ||
		errors.Is(err, ErrSelectorFeatureFlagMissing)
}
