package runtime

import (
	"context"
	"errors"
	"testing"

	provider "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/core"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/factory/llm"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
)

func TestModelTimeoutReasonsDoNotBecomeRunTimeout(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"queue_wait", llm.ErrModelQueueTimeout, "queue.timeout"},
		{"queue_full", errors.Join(llm.ErrPhysicalModelPoolUnavailable, agent_run.ErrModelQueueFull), "queue.full"},
		{"redis_unavailable", llm.ErrPhysicalModelPoolUnavailable, "store.unavailable"},
		{"provider_request", &provider.ProviderCallError{Cause: context.DeadlineExceeded}, "provider.timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := taskFailureReason(tc.err); got != tc.want {
				t.Fatalf("task reason=%q, want %q", got, tc.want)
			}
			if got := runtimeFailureOutcome("intent.detect_error", tc.err).ReasonCode; got != tc.want {
				t.Fatalf("Run reason=%q, want %q", got, tc.want)
			}
			verdict, err := (DeterministicExecutionVerifier{}).Verify(context.Background(), nil, nil, tc.err)
			if err != nil || verdict.ReasonCode != tc.want {
				t.Fatalf("verifier reason=%q err=%v", verdict.ReasonCode, err)
			}
		})
	}
}
