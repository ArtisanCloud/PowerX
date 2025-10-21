package workflow

import (
	"encoding/json"
	"time"

	"gorm.io/datatypes"
)

// decodeRetryPolicy 从 JSON 字段解析重试策略，若缺失则返回默认值。
func decodeRetryPolicy(data datatypes.JSON) RetryPolicyDefinition {
	policy := RetryPolicyDefinition{
		MaxAttempts:       3,
		InitialInterval:   30 * time.Second,
		BackoffMultiplier: 2.0,
		MaxInterval:       5 * time.Minute,
	}
	if len(data) == 0 {
		return policy
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return policy
	}
	if v := toInt(raw["max_attempts"]); v > 0 {
		policy.MaxAttempts = v
	}
	if v := toDuration(raw["initial_interval_ms"], time.Millisecond); v > 0 {
		policy.InitialInterval = v
	}
	if v := toFloat(raw["backoff_multiplier"]); v > 0 {
		policy.BackoffMultiplier = v
	}
	if v := toDuration(raw["max_interval_ms"], time.Millisecond); v > 0 {
		policy.MaxInterval = v
	}
	if v := toDuration(raw["jitter_ms"], time.Millisecond); v > 0 {
		policy.Jitter = v
	}
	return policy
}

func toInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case json.Number:
		i, _ := val.Int64()
		return int(i)
	}
	return 0
}

func toFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case json.Number:
		x, _ := val.Float64()
		return x
	}
	return 0
}

func toDuration(v any, unit time.Duration) time.Duration {
	switch val := v.(type) {
	case int:
		return time.Duration(val) * unit
	case int64:
		return time.Duration(val) * unit
	case float64:
		return time.Duration(val) * unit
	case json.Number:
		i, _ := val.Int64()
		return time.Duration(i) * unit
	}
	return 0
}
