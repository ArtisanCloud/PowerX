package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

type modelConcurrencyGate struct {
	limit  int
	active int
	wait   chan struct{}
}

var modelConcurrencyGates = struct {
	sync.Mutex
	items map[string]*modelConcurrencyGate
}{items: make(map[string]*modelConcurrencyGate)}

// acquireModelConcurrencyLease serializes only calls targeting the same
// tenant/environment/provider/model. Profile capacity is evaluated for every
// acquisition, so a published profile change applies to queued/new calls.
func acquireModelConcurrencyLease(ctx context.Context, mc *config.ModelConfig) (func(), error) {
	if mc == nil || mc.MaxConcurrentRequests <= 0 {
		return func() {}, nil
	}
	provider, model := strings.TrimSpace(mc.Provider), strings.TrimSpace(mc.Model)
	if provider == "" || model == "" {
		return nil, fmt.Errorf("model concurrency lease requires provider and model")
	}
	key := strings.Join([]string{
		firstNonEmpty(strings.TrimSpace(reqctx.GetTenantUUID(ctx)), "global"),
		firstNonEmpty(strings.TrimSpace(reqctx.GetEnv(ctx)), "default"),
		provider,
		model,
	}, "\x00")

	for {
		modelConcurrencyGates.Lock()
		gate := modelConcurrencyGates.items[key]
		if gate == nil {
			gate = &modelConcurrencyGate{limit: mc.MaxConcurrentRequests, wait: make(chan struct{})}
			modelConcurrencyGates.items[key] = gate
		} else if gate.limit != mc.MaxConcurrentRequests {
			gate.limit = mc.MaxConcurrentRequests
			close(gate.wait)
			gate.wait = make(chan struct{})
		}
		if gate.active < gate.limit {
			gate.active++
			modelConcurrencyGates.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					modelConcurrencyGates.Lock()
					gate.active--
					close(gate.wait)
					gate.wait = make(chan struct{})
					modelConcurrencyGates.Unlock()
				})
			}, nil
		}
		wait := gate.wait
		modelConcurrencyGates.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wait:
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
