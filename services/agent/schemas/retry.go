package schemas

import "time"

type RetryPolicy struct {
	ShouldRetry     bool
	MaxRetries      int
	BackoffDuration time.Duration
}
