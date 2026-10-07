package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// ProviderCallError records the provider-facing phase that failed. It is a
// transport contract, not user-visible copy: callers map it to their own UI.
type ProviderCallError struct {
	Provider       string
	Model          string
	Endpoint       string
	Phase          string
	Elapsed        time.Duration
	RequestTimeout time.Duration
	Cause          error
}

func (e *ProviderCallError) Error() string {
	if e == nil {
		return "llm provider call failed"
	}
	return fmt.Sprintf("llm provider call failed provider=%s model=%s phase=%s: %v", e.Provider, e.Model, e.Phase, e.Cause)
}

func (e *ProviderCallError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *ProviderCallError) IsTimeout() bool {
	if e == nil {
		return false
	}
	if errors.Is(e.Cause, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(e.Cause, &netErr) && netErr.Timeout()
}

func (e *ProviderCallError) Details() map[string]any {
	if e == nil {
		return nil
	}
	details := map[string]any{
		"provider":   strings.TrimSpace(e.Provider),
		"model":      strings.TrimSpace(e.Model),
		"phase":      strings.TrimSpace(e.Phase),
		"elapsed_ms": e.Elapsed.Milliseconds(),
	}
	if e.RequestTimeout > 0 {
		details["request_timeout_ms"] = e.RequestTimeout.Milliseconds()
	}
	if e.IsTimeout() {
		details["reason_code"] = "AI_PROVIDER_TIMEOUT"
	}
	return details
}
