package runtime

import (
	"errors"

	provider "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/core"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/factory/llm"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
)

var errAdminCapabilityContractChanged = errors.New("admin run capability contract changed")

func planningFailureReason(err error, fallback string) string {
	if reason := modelFailureReason(err); reason != "" {
		return reason
	}
	return fallback
}

// modelFailureReason keeps queue waiting, physical pool failures and provider
// request timeouts distinct from the enclosing Run deadline.
func modelFailureReason(err error) string {
	switch {
	case errors.Is(err, errAdminCapabilityContractChanged):
		return "authorization.contract_changed"
	case errors.Is(err, ErrRuntimeBudgetExhausted):
		return "budget.exhausted"
	case errors.Is(err, llm.ErrModelQueueTimeout):
		return "queue.timeout"
	case errors.Is(err, agent_run.ErrModelQueueFull):
		return "queue.full"
	case errors.Is(err, llm.ErrPhysicalModelPoolUnavailable), errors.Is(err, llm.ErrPhysicalModelPoolUnconfigured):
		return "store.unavailable"
	}
	var callErr *provider.ProviderCallError
	if errors.As(err, &callErr) && callErr.IsTimeout() {
		return "provider.timeout"
	}
	return ""
}
