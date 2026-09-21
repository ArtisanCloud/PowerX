package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

// CapabilityCompletionVerifier is a capability-owned, typed readback check.
// It must verify a real business object through its service boundary and return
// an immutable artifact reference; Runtime never infers completion from text.
type CapabilityCompletionVerifier interface {
	VerifyCompletion(context.Context, ResourceDescriptor, flowschema.PlanTask, *agentschema.ExecutionResult) (string, error)
}

type CompletionVerifierRegistry struct {
	mu             sync.RWMutex
	byCapabilityID map[string]CapabilityCompletionVerifier
}

func NewCompletionVerifierRegistry() *CompletionVerifierRegistry {
	return &CompletionVerifierRegistry{byCapabilityID: map[string]CapabilityCompletionVerifier{}}
}
func (r *CompletionVerifierRegistry) Register(capabilityID string, verifier CapabilityCompletionVerifier) error {
	if r == nil || verifier == nil || strings.TrimSpace(capabilityID) == "" {
		return fmt.Errorf("completion verifier registration is incomplete")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := strings.TrimSpace(capabilityID)
	if _, exists := r.byCapabilityID[key]; exists {
		return fmt.Errorf("completion verifier is already registered")
	}
	r.byCapabilityID[key] = verifier
	return nil
}
func (r *CompletionVerifierRegistry) Verify(ctx context.Context, resource ResourceDescriptor, task flowschema.PlanTask, out *agentschema.ExecutionResult) (string, error) {
	if r == nil || strings.TrimSpace(resource.CapabilityID) == "" {
		return "", fmt.Errorf("completion verifier is not configured")
	}
	r.mu.RLock()
	verifier := r.byCapabilityID[resource.CapabilityID]
	r.mu.RUnlock()
	if verifier == nil {
		return "", fmt.Errorf("completion verifier is not registered for capability")
	}
	return verifier.VerifyCompletion(ctx, resource, task, out)
}

type completionVerifierRegistryContextKey struct{}

func ContextWithCompletionVerifierRegistry(ctx context.Context, registry *CompletionVerifierRegistry) context.Context {
	if ctx == nil || registry == nil {
		return ctx
	}
	return context.WithValue(ctx, completionVerifierRegistryContextKey{}, registry)
}
func CompletionVerifierRegistryFromContext(ctx context.Context) (*CompletionVerifierRegistry, bool) {
	registry, ok := ctx.Value(completionVerifierRegistryContextKey{}).(*CompletionVerifierRegistry)
	return registry, ok && registry != nil
}
