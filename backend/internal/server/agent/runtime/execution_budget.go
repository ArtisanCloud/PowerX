package runtime

import (
	"fmt"
	"time"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

// runDurationForPlan validates the independent Run budget. Task count,
// concurrency waves, and provider request timeout never determine this limit.
func runDurationForPlan(plan flowschema.ExecutionPlan, runDeadline, requestTimeout time.Duration) (time.Duration, error) {
	if len(plan.Tasks) == 0 {
		return 0, fmt.Errorf("execution plan requires tasks")
	}
	if requestTimeout <= 0 {
		return 0, fmt.Errorf("LLM request timeout is required")
	}
	if runDeadline <= 0 {
		return 0, fmt.Errorf("Agent Run deadline is required")
	}
	return runDeadline, nil
}
