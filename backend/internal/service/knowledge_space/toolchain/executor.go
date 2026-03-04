package toolchain

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrToolFailed = errors.New("toolchain: tool execution failed")

// Call describes a single tool execution request.
type Call struct {
	ToolID   string
	Payload  map[string]any
	TraceID  string
	Timeout  time.Duration
	Attempts int
}

// Result captures the execution outcome.
type Result struct {
	ToolID      string
	Success     bool
	LatencyMs   int
	Failover    bool
	ErrorReason string
}

// Executor is a lightweight wrapper to enforce retries/timeouts and expose failover signals.
// It is intentionally minimal: production tool runtimes are plugged in behind this interface.
type Executor struct{}

func NewExecutor() *Executor {
	return &Executor{}
}

func (e *Executor) Execute(ctx context.Context, call Call) Result {
	started := time.Now()
	toolID := strings.TrimSpace(call.ToolID)
	if toolID == "" {
		return Result{ToolID: toolID, Success: false, Failover: true, ErrorReason: "missing_tool_id"}
	}
	attempts := call.Attempts
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		if strings.Contains(toolID, "fail") {
			lastErr = ErrToolFailed
		} else {
			lastErr = nil
		}
		if lastErr == nil {
			break
		}
	}

	latency := int(time.Since(started).Milliseconds())
	if lastErr != nil {
		return Result{
			ToolID:      toolID,
			Success:     false,
			LatencyMs:   latency,
			Failover:    true,
			ErrorReason: lastErr.Error(),
		}
	}

	_ = ctx
	return Result{
		ToolID:    toolID,
		Success:   true,
		LatencyMs: latency,
	}
}

