package schemas

import "time"

type RetryPolicy struct {
	MaxRetries     int           `json:"max_retries"`                // 最大重试次数
	Backoff        BackoffKind   `json:"backoff"`                    // 退避策略
	InitialDelay   time.Duration `json:"initial_delay,omitempty"`    // 首次重试延迟
	MaxDelay       time.Duration `json:"max_delay,omitempty"`        // 最大延迟
	RetryOnCodes   []string      `json:"retry_on_codes,omitempty"`   // 匹配错误码重试
	RetryOnMessage []string      `json:"retry_on_message,omitempty"` // 包含关键字重试
}
type BackoffKind string

const (
	BackoffNone        BackoffKind = "none"
	BackoffLinear      BackoffKind = "linear"
	BackoffExponential BackoffKind = "exponential"
)
