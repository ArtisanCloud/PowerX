package delivery

import (
	"context"
	"fmt"
	"strings"

	eventbus "github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// DriverFallbackPolicy 描述主驱动与降级驱动的路由策略。
type DriverFallbackPolicy struct {
	Primary  eventbus.QueueDriverType
	Fallback []eventbus.QueueDriverType
}

// DriverSelection 用于暴露当前可用驱动与降级决策结果。
type DriverSelection struct {
	Primary           eventbus.QueueDriverType
	FallbackCandidates []eventbus.QueueDriverType
	Available         map[eventbus.QueueDriverType]eventbus.QueueDriverCapability
}

// Normalize 将策略标准化，并去除重复/空值。
func (p DriverFallbackPolicy) Normalize() DriverFallbackPolicy {
	primary := normalizeDriverType(p.Primary)
	fallback := make([]eventbus.QueueDriverType, 0, len(p.Fallback))
	seen := map[eventbus.QueueDriverType]struct{}{}
	for _, item := range p.Fallback {
		driver := normalizeDriverType(item)
		if driver == "" || driver == primary {
			continue
		}
		if _, exists := seen[driver]; exists {
			continue
		}
		seen[driver] = struct{}{}
		fallback = append(fallback, driver)
	}
	return DriverFallbackPolicy{Primary: primary, Fallback: fallback}
}

// ResolveDriverSelection 根据可用驱动解析当前主/备方案。
func ResolveDriverSelection(policy DriverFallbackPolicy, drivers map[eventbus.QueueDriverType]eventbus.TaskDriver) (DriverSelection, error) {
	normalized := policy.Normalize()
	available := make(map[eventbus.QueueDriverType]eventbus.QueueDriverCapability)
	for driverType, driver := range drivers {
		if driver == nil {
			continue
		}
		normalizedType := normalizeDriverType(driverType)
		if normalizedType == "" {
			continue
		}
		available[normalizedType] = driver.Capability()
	}

	if normalized.Primary == "" {
		return DriverSelection{}, fmt.Errorf("primary driver is required")
	}
	if _, ok := available[normalized.Primary]; !ok {
		return DriverSelection{}, fmt.Errorf("primary driver %s is unavailable", normalized.Primary)
	}

	fallback := make([]eventbus.QueueDriverType, 0, len(normalized.Fallback))
	for _, candidate := range normalized.Fallback {
		if _, ok := available[candidate]; ok {
			fallback = append(fallback, candidate)
		}
	}

	return DriverSelection{
		Primary:            normalized.Primary,
		FallbackCandidates: fallback,
		Available:          available,
	}, nil
}

// TryOnDriver 优先主驱动执行，失败时按 fallback 顺序尝试。
func TryOnDriver(ctx context.Context, selection DriverSelection, drivers map[eventbus.QueueDriverType]eventbus.TaskDriver, fn func(context.Context, eventbus.TaskDriver) error) error {
	run := func(driverType eventbus.QueueDriverType) error {
		driver := drivers[driverType]
		if driver == nil {
			return fmt.Errorf("driver %s is unavailable", driverType)
		}
		return fn(ctx, driver)
	}

	primaryErr := run(selection.Primary)
	if primaryErr == nil {
		return nil
	}

	var lastErr error
	for _, candidate := range selection.FallbackCandidates {
		if err := run(candidate); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("driver %s execution failed without fallback: %w", selection.Primary, primaryErr)
}

func normalizeDriverType(driver eventbus.QueueDriverType) eventbus.QueueDriverType {
	value := strings.ToLower(strings.TrimSpace(string(driver)))
	if value == "" {
		return ""
	}
	return eventbus.QueueDriverType(value)
}
